import SwiftUI
import AppKit
import Security

@MainActor final class Model: ObservableObject {
 @Published var password="", hub="", code="", interval=10, processes=false, addresses=false, hostname=false, battery=false
 @Published var status="本机采集 · 主动上报", notice="", unlocked=false, busy=false, paused=false
 private var config:[String:Any]=[:], csrf=""
 private let base="http://127.0.0.1:17860"
 func request(_ path:String,_ body:[String:Any]?=nil) async throws -> [String:Any] {
  var r=URLRequest(url:URL(string:base+path)!);r.timeoutInterval=20
  if let body {r.httpMethod="POST";r.httpBody=try JSONSerialization.data(withJSONObject:body);r.setValue("application/json",forHTTPHeaderField:"Content-Type");r.setValue(base,forHTTPHeaderField:"Origin");r.setValue(csrf,forHTTPHeaderField:"X-CSRF-Token")}
  let(data,response)=try await URLSession.shared.data(for:r)
  guard let http=response as? HTTPURLResponse,http.statusCode==200 else {throw NSError(domain:"Chronos",code:1,userInfo:[NSLocalizedDescriptionKey:"操作未完成，请检查密码、配对码与连接状态"])}
  return try JSONSerialization.jsonObject(with:data) as? [String:Any] ?? [:]
 }
 func run(_ action:() async throws -> Void) async {guard !busy else{return};busy=true;defer{busy=false};do{try await action()}catch{notice=error.localizedDescription}}
 func login() async {await run {
  let s=try await request("/api/session");var body:[String:Any]=["password":password];var path="/api/login"
  if !(s["initialized"] as? Bool ?? false){path="/api/setup";body["bootstrap"]=try String(contentsOf:FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/ChronosAgents/bootstrap-secret"),encoding:.utf8).trimmingCharacters(in:.whitespacesAndNewlines)}
  let result=try await request(path,body);csrf=result["csrf"] as? String ?? "";remember(password);password="";unlocked=true;try await refresh(true);notice="已解锁，关闭窗口后在菜单栏继续运行"
 }}
 func refresh(_ fields:Bool=false) async throws {
  let value=try await request("/api/local/status");config=value["config"] as? [String:Any] ?? [:];let s=value["status"] as? [String:Any] ?? [:]
  status="状态：\(s["state"] ?? "未知") · 成功上报 \(s["successes"] ?? 0) 次";paused=config["paused"] as? Bool ?? false
  if fields {hub=config["hub"] as? String ?? "";interval=config["interval"] as? Int ?? 10;processes=config["processes"] as? Bool ?? false;addresses=config["addresses"] as? Bool ?? false;hostname=config["hostname"] as? Bool ?? false;battery=config["pause_on_battery"] as? Bool ?? false}
 }
 func save() async {await run{var c=config;c["interval"]=interval;c["processes"]=processes;c["addresses"]=addresses;c["hostname"]=hostname;c["pause_on_battery"]=battery;_=try await request("/api/local/config",c);try await refresh();notice="设置已保存"}}
 func pair() async {await run{_=try await request("/api/local/pair",["hub":hub,"code":code,"ca_pem":config["ca_pem"] ?? ""]);code="";try await refresh(true)}}
 func toggle() async {await run{var c=config;c["paused"] = !paused;_=try await request("/api/local/config",c);try await refresh()}}
 func upload() async {await run{_=try await request("/api/local/upload",[:]);notice="已请求采样；重试退避仍然生效"}}
 private var key:[String:Any]{[kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:"dev.chronos.agents.native",kSecAttrAccount as String:NSUserName()]}
 private func remember(_ p:String){SecItemDelete(key as CFDictionary);var q=key;q[kSecValueData as String]=Data(p.utf8);q[kSecAttrAccessible as String]=kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly;SecItemAdd(q as CFDictionary,nil)}
 func restore() async {var q=key;q[kSecReturnData as String]=true;var result:CFTypeRef?;if SecItemCopyMatching(q as CFDictionary,&result)==errSecSuccess,let d=result as? Data,let p=String(data:d,encoding:.utf8){password=p;await login()}}
}
struct ControlView: View {
 @StateObject private var model=Model()
 var body:some View {
  VStack(alignment:.leading,spacing:18){
   Label("Chronos Agent",systemImage:"cpu").font(.largeTitle.bold())
   Text(model.status).foregroundStyle(.secondary)
   HStack{SecureField("本机管理密码",text:$model.password);Button("解锁"){Task{await model.login()}}}.textFieldStyle(.roundedBorder)
   Divider()
   Group {
    TextField("Hub HTTPS 地址",text:$model.hub)
    HStack{TextField("一次性配对码",text:$model.code);Button("配对设备"){Task{await model.pair()}}}
    Stepper("采样间隔：\(model.interval) 秒",value:$model.interval,in:10...300)
    Toggle("上传进程",isOn:$model.processes);Toggle("上传网卡地址",isOn:$model.addresses);Toggle("上传主机名",isOn:$model.hostname);Toggle("电池供电时暂停",isOn:$model.battery)
    HStack{Button("保存设置"){Task{await model.save()}};Button(model.paused ? "恢复上传":"暂停上传"){Task{await model.toggle()}};Button("立即上传"){Task{await model.upload()}}}
   }.disabled(!model.unlocked || model.busy)
   Text(model.notice).font(.callout).foregroundStyle(.secondary).frame(minHeight:40,alignment:.topLeading)
   Text("凭据保存在 macOS 钥匙串 · 后台启动由配套安装脚本配置").font(.caption).foregroundStyle(.secondary)
  }.padding(28).frame(width:540).background(LinearGradient(colors:[Color(red:0.89,green:0.96,blue:0.95),Color(red:0.94,green:0.91,blue:0.98)],startPoint:.topLeading,endPoint:.bottomTrailing)).textFieldStyle(.roundedBorder)
   .onAppear{NSApp.setActivationPolicy(.regular);NSApp.activate(ignoringOtherApps:true)}
   .task{await model.restore();while !Task.isCancelled {try? await Task.sleep(nanoseconds:5_000_000_000);if model.unlocked && !model.busy{try? await model.refresh()}}}
 }
}
@main struct NativeControl: App {var body:some Scene{WindowGroup("Chronos Agent"){ControlView()}.windowResizability(.contentSize)}}
