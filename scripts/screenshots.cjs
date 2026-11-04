// Fictional screenshot fixtures only. Never connect to a real user's Hub.
// npm install --no-save playwright; node scripts/screenshots.cjs
const { chromium } = require(process.env.CHRONOS_PLAYWRIGHT_PATH || "playwright");
const fs = require("node:fs");
const http = require("node:http");
const path = require("node:path");
const root = path.resolve(__dirname, "..");
const stamp = new Date().toISOString();
const block = (data, scope = "host") => ({status: "supported", scope, collected_at: stamp, data});
function device(id, name, kind, cpu, mem) {
  return {id, name, kind, created: 1767225600, received_at: Date.now()/1000, state: "recent", revoked: false,
    policy: {interval: 10, processes: false, addresses: false, hostname: false, paused: false},
    latest: {schema_version: 1, device_id: id, agent_version: "0.1.0", session_id: "fictional_demo_session", sequence: 20,
      sampled_at: stamp, expected_interval_seconds: 10, platform: {os: kind, arch: "amd64", hostname: ""},
      metrics: {cpu: block({percent: cpu, cores: 8}), memory: block({percent: mem, total_bytes: 17179869184}),
        network: block({rx_bytes_per_second: 163840, tx_bytes_per_second: 12288}),
        storage: block({volumes: [{name: "data", percent: 42, total_bytes: 1024000000000}]}),
        processes: {status: "disabled", scope: "host", collected_at: stamp, data: null},
        sensors: {status: "unsupported", scope: "host", collected_at: stamp, data: null}}}};
}
const devices = [device("demo_cloud_001", "云服务器", "linux", 8.4, 38.2), device("demo_nas_002", "家庭 NAS", "nas", 4.2, 26.5), device("demo_work_003", "工作电脑", "windows", 15.6, 42.8)];
const server = http.createServer((req, res) => {
  const names = {"/": "index.html", "/app.js": "app.js", "/style.css": "style.css", "/logo.svg": "logo.svg"};
  const file = names[req.url]; if (!file) {res.writeHead(404);res.end();return;}
  res.setHeader("Content-Type", {"index.html":"text/html; charset=utf-8", "app.js":"application/javascript; charset=utf-8", "style.css":"text/css", "logo.svg":"image/svg+xml"}[file]);
  res.end(fs.readFileSync(path.join(root, "web/ui", file)));
});
(async () => {
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const browser = await chromium.launch({headless: true});
  const page = await browser.newPage({viewport: {width: 1440, height: 1000}, deviceScaleFactor: 1});
  let role = "hub";
  await page.route("**/api/**", async route => {
    const url = new URL(route.request().url()); let data = {};
    switch (url.pathname) {
      case "/api/info": data = {role, version:"0.1.0 · 演示"}; break;
      case "/api/session": data = {authenticated:true, initialized:true, csrf:"fictional-demo"};break;
      case "/api/v1/devices": data = {items:devices};break;
      case "/api/local/status": data = {config:{hub:"https://hub.example.com",device_id:"demo_work_003",interval:10,timeout:12,processes:false,addresses:false,hostname:false,paused:false,pause_on_battery:false,ca_pem:""},status:{state:"online",last_success:Date.now()/1000,successes:24,failures:0,queued:0,dropped:0,credential_storage:"Windows DPAPI",center_policy:devices[0].policy,sample:devices[2].latest}};break;
      default: data = {items:[]};
    }
    await route.fulfill({json:data});
  });
  const output = path.join(root,"docs/screenshots");fs.mkdirSync(output,{recursive:true});
  for (const name of ["hub","agent"]) {
    role=name;
    await page.goto(`http://127.0.0.1:${server.address().port}`);
    await page.locator(name==="hub"?".device-card":"#local-state").first().waitFor();
    await page.locator("#role-label").evaluate(e=>e.textContent="虚构演示 · 不连接真实设备");
    for (const width of [1440,390]) {
      await page.setViewportSize({width,height:1000});
      await page.screenshot({path:path.join(output,`${name}-${width}.png`),fullPage:true});
    }
  }
  await browser.close();server.close();
})().catch(error=>{console.error(error.message);server.close();process.exit(1);});
