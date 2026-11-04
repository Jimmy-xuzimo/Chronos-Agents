const $ = (id) => document.getElementById(id);
let role = "",
  csrf = "",
  initialized = false,
  page = "devices",
  devices = [],
  localConfig = null,
  timer;
const labels = {
  cpu: "CPU",
  memory: "内存",
  swap: "Swap",
  storage: "存储",
  network: "网络",
  system: "系统",
  processes: "进程",
  battery: "电池",
  sensors: "传感器",
};
const states = {
  supported: "可用",
  unsupported: "不支持",
  permission_denied: "权限不足",
  disabled: "已禁用",
  error: "采集异常",
  warming_up: "建立采样基线",
  online: "上报正常",
  recent: "最近已上报",
  stale: "长时间未上报",
  sync_delayed: "同步延迟",
  paused: "已暂停",
  center_paused: "中心暂停",
  battery_paused: "电池暂停",
  credential_revoked: "凭据失效",
  unpaired: "尚未配对",
  awaiting_enrollment: "等待配对",
  revoked: "已撤销",
  paired: "已配对",
  retrying: "等待重试",
};
function node(tag, text, cls) {
  const e = document.createElement(tag);
  if (text !== undefined) e.textContent = text;
  if (cls) e.className = cls;
  return e;
}
function notice(text, error = false) {
  $("notice").textContent = text;
  $("notice").className = error ? "error" : "";
  $("notice").hidden = false;
  clearTimeout(notice.timeout);
  notice.timeout = setTimeout(() => ($("notice").hidden = true), 8000);
}
async function api(path, body) {
  const options = {
    credentials: "same-origin",
    cache: "no-store",
    headers: {},
  };
  if (body !== undefined) {
    options.method = "POST";
    options.headers = {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
    };
    options.body = JSON.stringify(body);
  }
  const r = await fetch(path, options);
  const data = await r.json();
  if (!r.ok) {
    if (r.status === 401 && path !== "/api/login") await session();
    throw Error(data.error || "请求失败");
  }
  return data;
}
function bind(id, fn) {
  $(id).addEventListener("click", () =>
    Promise.resolve()
      .then(fn)
      .catch((e) => notice(e.message, true)),
  );
}
function form(id, fn) {
  $(id).addEventListener("submit", async (e) => {
    e.preventDefault();
    const b = e.submitter;
    if (b) b.disabled = true;
    try {
      await fn();
    } catch (error) {
      notice(error.message, true);
    } finally {
      if (b) b.disabled = false;
    }
  });
}
function modal(title, content) {
  $("modal-title").textContent = title;
  $("modal-body").replaceChildren(...content);
  $("modal").showModal();
}
function sensitive(title, value, extra = []) {
  const box = node("div", value, "secret");
  const copy = node("button", "复制");
  copy.addEventListener("click", () =>
    navigator.clipboard
      .writeText(value)
      .then(() => notice("已复制，请妥善保存"))
      .catch(() => notice("请手动选择并复制")),
  );
  modal(title, [
    node("p", "仅显示这一次，不要分享或放入公开截图。"),
    ...extra,
    box,
    copy,
  ]);
}
function percent(v) {
  return typeof v === "number" ? v.toFixed(1).replace(/\.0$/, "") + "%" : "—";
}
function bytes(v) {
  if (typeof v !== "number") return "—";
  let i = 0;
  const u = ["B", "KB", "MB", "GB", "TB"];
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return v.toFixed(i > 1 ? 1 : 0) + " " + u[i];
}
function metric(sample, name, key) {
  const m = sample?.metrics?.[name];
  return m?.status === "supported" ? m.data?.[key] : null;
}
function at(ts) {
  return ts
    ? new Date(typeof ts === "number" ? ts * 1000 : ts).toLocaleString(
        "zh-CN",
        { hour12: false },
      )
    : "尚无数据";
}
async function session() {
  const s = await api("/api/session");
  csrf = s.csrf || "";
  initialized = s.initialized;
  $("auth").hidden = s.authenticated;
  $("workspace").hidden = !s.authenticated;
  $("logout").hidden = !s.authenticated;
  $("bootstrap-label").hidden = initialized;
  $("totp-label").hidden = !initialized;
  $("auth-title").textContent = initialized
    ? "回到设备工作空间。"
    : "建立你的设备工作空间。";
  $("auth-help").textContent = initialized
    ? "使用本服务管理密码登录。"
    : "使用私有目录中的一次性凭据，设置至少 12 位的管理密码。";
  $("login-submit").textContent = initialized ? "进入管理" : "初始化并进入";
  $("password").autocomplete = initialized
    ? "current-password"
    : "new-password";
  if (s.authenticated) {
    $("hub-nav").hidden = role !== "hub";
    $("local").hidden = role !== "agent";
    if (role === "hub") await navigate(page);
    else await refreshLocal(true);
  }
  return s.authenticated;
}
async function navigate(next) {
  page = next;
  document
    .querySelectorAll("[data-page]")
    .forEach((b) => b.classList.toggle("selected", b.dataset.page === page));
  for (const p of ["devices", "connections", "history", "settings"])
    $("hub-" + p).hidden = p !== page;
  if (page === "devices" || page === "connections" || page === "history")
    await refreshDevices();
  if (page === "connections") await refreshReaders();
  if (page === "settings") {
    const s = await api("/api/admin/settings");
    $("raw-hours").value = s.raw_hours;
    $("minute-days").value = s.minute_days;
    $("audit-days").value = s.audit_days;
    $("mfa-status").textContent = s.totp_enabled
      ? "两步认证已启用"
      : "两步认证尚未启用";
  }
  if (page === "history") {
    await history();
    const data = await api("/api/admin/audit");
    $("audit").replaceChildren(
      ...data.items.map((row) => {
        const e = node("div", undefined, "row");
        e.append(
          node("span", row.event),
          node("small", at(row.at) + " · " + (row.device_id || "管理操作")),
        );
        return e;
      }),
    );
  }
}
async function refreshDevices() {
  const result = await api("/api/v1/devices");
  devices = result.items;
  $("fleet-summary").textContent =
    `${devices.length} 台设备 · ${devices.filter((d) => d.state === "recent").length} 台最近上报`;
  if (page === "devices")
    $("devices").replaceChildren(
      ...(devices.length
        ? devices.map(deviceCard)
        : [
            node(
              "p",
              "还没有设备。添加设备后，在采集器内输入一次性配对码。",
              "empty",
            ),
          ]),
    );
  if (page === "connections") {
    $("reader-scopes").replaceChildren(
      ...devices.map((d) => {
        const l = node("label", undefined, "check");
        const c = node("input");
        c.type = "checkbox";
        c.value = d.id;
        l.append(c, document.createTextNode(d.name));
        return l;
      }),
    );
  }
  if (page === "history") {
    const selected = $("history-device").value;
    $("history-device").replaceChildren(
      ...devices.map((d) => {
        const o = node("option", d.name);
        o.value = d.id;
        return o;
      }),
    );
    if (devices.some((d) => d.id === selected))
      $("history-device").value = selected;
  }
}
function deviceCard(d) {
  const card = node("article", undefined, "glass device-card");
  const top = node("div", undefined, "heading");
  top.append(
    node("h3", d.name),
    node("span", states[d.state] || d.state, "badge"),
  );
  card.append(top, node("div", d.kind + " / " + d.id, "device-id"));
  const metrics = node("div", undefined, "metrics");
  for (const [label, v] of [
    ["CPU", percent(metric(d.latest, "cpu", "percent"))],
    ["内存", percent(metric(d.latest, "memory", "percent"))],
    ["下载", typeof metric(d.latest,"network","rx_bytes_per_second")==="number" ? bytes(metric(d.latest, "network", "rx_bytes_per_second"))+"/s" : "—"],
  ]) {
    const x = node("div");
    x.append(node("small", label), node("strong", v));
    metrics.append(x);
  }
  card.append(
    metrics,
    node("small", "采样 " + at(d.latest?.sampled_at)),
    node("small", "收到 " + at(d.received_at)),
  );
  const actions = node("div", undefined, "actions");
  const detail = node("button", "能力与详情");
  detail.onclick = () => details(d);
  const policy = node("button", "采集权限");
  policy.onclick = () => policyDialog(d);
  const revoke = node("button", "撤销");
  revoke.className = "danger";
  revoke.onclick = async () => {
    if (!confirm("撤销 " + d.name + " 的设备凭据？现有连接会被拒绝。")) return;
    try {
      await api("/api/admin/revoke", { id: d.id });
      await refreshDevices();
    } catch (e) {
      notice(e.message, true);
    }
  };
  const repair = node("button", "重新配对");
  repair.onclick = () =>
    createPair({ id: d.id }).catch((e) => notice(e.message, true));
  actions.append(detail, policy, repair, revoke);
  card.append(actions);
  return card;
}
function details(d) {
  const caps = node("div", undefined, "cap-grid");
  for (const [key, m] of Object.entries(d.latest?.metrics || {})) {
    const c = node(
      "div",
      `${labels[key] || key} · ${states[m.status] || m.status}`,
      "cap",
    );
    c.append(node("small", m.scope + " · " + at(m.collected_at)));
    caps.append(c);
  }
  const pre = node(
    "pre",
    d.latest ? JSON.stringify(d.latest, null, 2) : "尚无样本",
  );
  modal(d.name, [
    node("p", "能力、范围和采样时间来自设备；历史不保留进程明细。"),
    caps,
    pre,
  ]);
}
function policyDialog(d) {
  const f = node("form");
  const interval = node("input");
  interval.type = "number";
  interval.min = 10;
  interval.max = 86400;
  interval.value = d.policy.interval || 10;
  const l = node("label", "最短采样间隔 / 秒");
  l.append(interval);
  f.append(l);
  const controls = {};
  for (const [k, text] of [
    ["paused", "中心暂停采样"],
    ["processes", "允许进程明细"],
    ["addresses", "允许网卡地址"],
    ["hostname", "允许主机名"],
  ]) {
    const box = node("input");
    box.type = "checkbox";
    box.checked = !!d.policy[k];
    const row = node("label", undefined, "check");
    row.append(box, document.createTextNode(text));
    controls[k] = box;
    f.append(row);
  }
  f.append(node("p", "客户端本地开关与中心权限同时允许时才上传敏感字段。"));
  const save = node("button", "保存权限", "primary");
  f.append(save);
  f.onsubmit = async (e) => {
    e.preventDefault();
    const p = { interval: Number(interval.value) };
    for (const [k, b] of Object.entries(controls)) p[k] = b.checked;
    try {
      await api("/api/admin/policy", { id: d.id, policy: p });
      $("modal").close();
      await refreshDevices();
    } catch (e) {
      notice(e.message, true);
    }
  };
  modal("采集权限 · " + d.name, [f]);
}
async function createPair(request) {
  const data = await api("/api/admin/pair", request);
  const image = node("img");
  image.className = "qr";
  image.src = data.qr;
  image.alt = "一次性设备配对二维码";
  sensitive("一次性配对 · 10 分钟有效", data.code, [
    image,
    node("p", "Hub 地址：" + location.origin),
  ]);
  await refreshDevices();
}
async function refreshReaders() {
  const data = await api("/api/admin/readers");
  $("readers").replaceChildren(
    ...data.items.map((reader) => {
      const row = node("div", undefined, "row");
      row.append(
        node("span", reader.name + " · " + reader.devices.length + " 台设备"),
      );
      const b = node("button", "撤销");
      b.onclick = async () => {
        try {
          await api("/api/admin/readers", { revoke: reader.id });
          await refreshReaders();
        } catch (e) {
          notice(e.message, true);
        }
      };
      row.append(b);
      return row;
    }),
  );
}
async function history() {
  const id = $("history-device").value;
  if (!id) {
    $("history-chart").textContent = "暂无设备";
    return;
  }
  const data = await api(
    "/api/v1/history?device=" +
      encodeURIComponent(id) +
      "&range=" +
      $("history-range").value,
  );
  const points = data.points.filter((p) => typeof p.cpu === "number");
  $("history-chart").replaceChildren();
  if (points.length < 2) {
    $("history-chart").append(
      node("p", "数据正在积累，至少两次有效 CPU 采样后显示曲线。", "empty"),
    );
  } else {
    const ns = "http://www.w3.org/2000/svg",
      svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 1000 220");
    svg.setAttribute("aria-label", "CPU 使用率历史，范围 0 到 100%");
    for (const y of [10, 60, 110, 160, 210]) {
      const line = document.createElementNS(ns, "line");
      line.setAttribute("x1", "0");
      line.setAttribute("x2", "1000");
      line.setAttribute("y1", y);
      line.setAttribute("y2", y);
      svg.append(line);
    }
    const line = document.createElementNS(ns, "polyline");
    line.setAttribute(
      "points",
      points
        .map(
          (p, i) =>
            `${(i / (points.length - 1)) * 1000},${210 - Math.max(0, Math.min(100, p.cpu)) * 2}`,
        )
        .join(" "),
    );
    svg.append(line);
    $("history-chart").append(svg);
  }
  $("history-info").textContent =
    `${data.points.length} 个汇总点 · 分辨率 ${data.resolution_seconds} 秒 · CPU 0–100%`;
}
async function refreshLocal(fill = false) {
  const data = await api("/api/local/status");
  localConfig = data.config;
  const s = data.status;
  $("local-state").textContent =
    `${states[s.state] || s.state} · 最近成功 ${at(s.last_success)} · 待发送 ${s.queued} 条`;
  $("cpu").textContent = percent(metric(s.sample, "cpu", "percent"));
  $("memory").textContent = percent(metric(s.sample, "memory", "percent"));
  const n = metric(s.sample, "network", "rx_bytes_per_second");
  $("network").textContent = typeof n === "number" ? bytes(n) + "/s" : "—";
  if (fill) {
    $("hub-origin").value = localConfig.hub || "";
    $("ca-pem").value = localConfig.ca_pem || "";
    $("interval").value = localConfig.interval;
    $("timeout").value = localConfig.timeout;
    $("paused").checked = localConfig.paused;
    $("battery").checked = localConfig.pause_on_battery;
    $("processes").checked = localConfig.processes;
    $("addresses").checked = localConfig.addresses;
    $("hostname").checked = localConfig.hostname;
  }
  $("local-binding").textContent = localConfig.device_id
    ? "设备 ID：" + localConfig.device_id
    : "尚未配对接收中心";
  $("local-security").textContent =
    "凭据存储：" +
    s.credential_storage +
    "。成功 " +
    s.successes +
    " 次，失败 " +
    s.failures +
    " 次，丢弃 " +
    s.dropped +
    " 条。" +
    (s.last_error ? "状态：" + s.last_error : "");
  $("local-policy").textContent =
    "中心权限：进程 " +
    (s.center_policy.processes ? "允许" : "关闭") +
    " / 地址 " +
    (s.center_policy.addresses ? "允许" : "关闭") +
    " / 主机名 " +
    (s.center_policy.hostname ? "允许" : "关闭");
  $("capabilities").replaceChildren(
    ...Object.entries(s.sample?.metrics || {}).map(([name, m]) => {
      const c = node(
        "div",
        (labels[name] || name) + " · " + (states[m.status] || m.status),
        "cap",
      );
      c.append(node("small", m.scope + " · " + at(m.collected_at)));
      return c;
    }),
  );
  if (!$("sample").hidden)
    $("sample").textContent = JSON.stringify(s.sample, null, 2);
}
form("login", async () => {
  const data = await api(initialized ? "/api/login" : "/api/setup", {
    password: $("password").value,
    bootstrap: $("bootstrap").value,
    totp: $("totp").value,
  });
  csrf = data.csrf;
  $("password").value = "";
  $("bootstrap").value = "";
  $("totp").value = "";
  await session();
});
bind("logout", async () => {
  await api("/api/logout", {});
  csrf = "";
  await session();
});
bind("close-modal", () => $("modal").close());
$("modal").addEventListener("close", () => $("modal-body").replaceChildren());
document
  .querySelectorAll("[data-page]")
  .forEach((b) =>
    b.addEventListener("click", () =>
      navigate(b.dataset.page).catch((e) => notice(e.message, true)),
    ),
  );
bind("new-device", () => navigate("connections"));
form("pair-form", () =>
  createPair({ name: $("device-name").value, kind: $("device-kind").value }),
);
form("reader-form", async () => {
  const devices = [...$("reader-scopes").querySelectorAll("input:checked")].map(
    (b) => b.value,
  );
  const result = await api("/api/admin/readers", {
    name: $("reader-name").value,
    devices,
  });
  sensitive("只读服务凭据", result.token);
  await refreshReaders();
});
bind("load-history", history);
form("retention-form", async () => {
  await api("/api/admin/settings", {
    raw_hours: Number($("raw-hours").value),
    minute_days: Number($("minute-days").value),
    audit_days: Number($("audit-days").value),
  });
  notice("保留期限已保存");
});
const reauth = () => ({
  password: $("old-password").value,
  totp: $("old-totp").value,
});
form("account-form", async () => {
  await api("/api/admin/account", {
    ...reauth(),
    action: "password",
    new_password: $("new-password").value,
  });
  $("old-password").value = "";
  $("new-password").value = "";
  await session();
});
bind("totp-prepare", async () => {
  const credentials = reauth(),
    data = await api("/api/admin/account", {
      ...credentials,
      action: "totp_prepare",
    });
  const f = node("form");
  f.append(
    node("p", "在验证器中添加此密钥，再输入生成的六位验证码。"),
    node("div", data.secret, "secret"),
  );
  const code = node("input");
  code.inputMode = "numeric";
  code.maxLength = 6;
  code.required = true;
  const l = node("label", "新验证码");
  l.append(code);
  f.append(l, node("button", "验证并启用", "primary"));
  f.onsubmit = async (e) => {
    e.preventDefault();
    try {
      await api("/api/admin/account", {
        ...credentials,
        action: "totp_enable",
        secret: data.secret,
        new_password: code.value,
      });
      $("modal").close();
      $("old-password").value = "";
      await session();
    } catch (error) {
      notice(error.message, true);
    }
  };
  modal("启用两步认证", [f]);
});
bind("totp-disable", async () => {
  await api("/api/admin/account", { ...reauth(), action: "totp_disable" });
  $("old-password").value = "";
  await session();
});
form("enroll-form", async () => {
  await api("/api/local/pair", {
    hub: $("hub-origin").value,
    code: $("pair-code").value,
    ca_pem: $("ca-pem").value,
  });
  $("pair-code").value = "";
  notice("设备已配对，等待首次上报");
  await refreshLocal(true);
});
form("local-config", async () => {
  await api("/api/local/config", {
    ...localConfig,
    interval: Number($("interval").value),
    timeout: Number($("timeout").value),
    paused: $("paused").checked,
    pause_on_battery: $("battery").checked,
    processes: $("processes").checked,
    addresses: $("addresses").checked,
    hostname: $("hostname").checked,
  });
  notice("采集设置已保存");
  await refreshLocal(true);
});
bind("upload-now", async () => {
  await api("/api/local/upload", {});
  notice("已安排采样；暂停时不会强行上传");
});
bind("rotate", async () => {
  await api("/api/local/rotate", {});
  notice("设备凭据已轮换");
});
bind("forget", async () => {
  if (confirm("清除本机连接？Hub 上的历史仍保留。")) {
    await api("/api/local/forget", {});
    await refreshLocal(true);
  }
});
bind("show-sample", async () => {
  $("sample").hidden = !$("sample").hidden;
  await refreshLocal();
});
async function poll() {
  clearTimeout(timer);
  try {
    if (!document.hidden && !$("workspace").hidden) {
      if (role === "agent") await refreshLocal();
      else if (page === "devices") await refreshDevices();
    }
  } catch (e) {
    notice(e.message, true);
  }
  timer = setTimeout(poll, role === "agent" ? 5000 : 10000);
}
document.addEventListener("visibilitychange", () => {
  if (!document.hidden) poll();
});
(async () => {
  try {
    const info = await api("/api/info");
    role = info.role;
    $("role-label").textContent =
      role === "hub" ? "统一设备接收中心" : "轻量设备采集器";
    $("connection").textContent =
      role === "hub" ? "Hub · 0.1.0" : "Agent · 0.1.0";
    await session();
    poll();
  } catch (e) {
    notice("服务连接失败：" + e.message, true);
  }
})();
