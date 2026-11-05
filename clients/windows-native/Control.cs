using System;
using System.IO;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Net;
using System.Net.Http;
using System.Collections.Generic;
using System.Web.Script.Serialization;
using System.Security.Cryptography;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using Microsoft.Win32;
using System.Runtime.InteropServices;

static class NativeApp {
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern IntPtr FindWindow(string cls,string title);
 [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr hwnd);
 [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr hwnd,int cmd);
 [STAThread] static void Main(){Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);using(var mutex=new System.Threading.Mutex(false,"Local\\ChronosAgentNativeUI")){if(!mutex.WaitOne(0,false)){var window=FindWindow(null,"Chronos Agent · 设备采集器");ShowWindow(window,9);SetForegroundWindow(window);return;}Application.Run(new ControlWindow());}}
}
class Surface : Panel {
 protected override void OnPaintBackground(PaintEventArgs e){using(var b=new LinearGradientBrush(ClientRectangle,Color.FromArgb(233,246,246),Color.FromArgb(239,232,249),30f)){e.Graphics.FillRectangle(b,ClientRectangle);}}
}
class ControlWindow : Form {
 readonly bool demo=Environment.GetEnvironmentVariable("CHRONOS_NATIVE_DEMO")=="1";
 readonly HttpClient client;readonly JavaScriptSerializer json=new JavaScriptSerializer();
 readonly string data=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),"ChronosAgents");
 TextBox password=new TextBox(),hub=new TextBox(),code=new TextBox();NumericUpDown interval=new NumericUpDown();
 Label status=new Label(),metrics=new Label(),notice=new Label();CheckBox processes=new CheckBox(),addresses=new CheckBox(),hostname=new CheckBox(),battery=new CheckBox(),startup=new CheckBox();
 Button login,save,pair,pause,upload;Dictionary<string,object> config;string csrf="";bool busy=false,paused=false;Timer timer=new Timer();
 Dictionary<string,object> Dict(object x){return x as Dictionary<string,object> ?? new Dictionary<string,object>();}
 string Str(Dictionary<string,object> d,string k){return d.ContainsKey(k)&&d[k]!=null?Convert.ToString(d[k]):"";}
 bool Flag(Dictionary<string,object>d,string k){return d.ContainsKey(k)&&d[k] is bool&&(bool)d[k];}
 public ControlWindow(){
 Text="Chronos Agent · 设备采集器"+(demo?" · 虚构演示":"");ClientSize=new Size(680,630);MinimumSize=MaximumSize=Size;MaximizeBox=false;StartPosition=FormStartPosition.CenterScreen;AutoScaleMode=AutoScaleMode.Dpi;Font=new Font("Microsoft YaHei UI",10);try{Icon=Icon.ExtractAssociatedIcon(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"chronos-agent.exe"));}catch{}
 var panel=new Surface{Dock=DockStyle.Fill,Padding=new Padding(28)};Controls.Add(panel);
 var heading=new Label{Text="Chronos Agent",Font=new Font(Font.FontFamily,24,FontStyle.Bold),Location=new Point(28,22),Size=new Size(600,48),BackColor=Color.Transparent};panel.Controls.Add(heading);
 status.Location=new Point(30,80);status.Size=new Size(610,26);status.BackColor=Color.Transparent;status.Text="本机采集 · 主动上报";panel.Controls.Add(status);
 metrics.Location=new Point(30,112);metrics.Size=new Size(610,28);metrics.Font=new Font(Font.FontFamily,12,FontStyle.Bold);metrics.BackColor=Color.Transparent;panel.Controls.Add(metrics);
 LabelAt(panel,"管理密码",30,158);password.Location=new Point(155,154);password.Size=new Size(330,30);password.UseSystemPasswordChar=true;panel.Controls.Add(password);login=ButtonAt(panel,"解锁",500,151,135,async()=>await Login());
 LabelAt(panel,"接收中心",30,214);hub.Location=new Point(155,210);hub.Size=new Size(480,30);panel.Controls.Add(hub);
 LabelAt(panel,"一次性配对码",30,258);code.Location=new Point(155,254);code.Size=new Size(330,30);panel.Controls.Add(code);pair=ButtonAt(panel,"配对设备",500,251,135,async()=>{await Api("/api/local/pair",new{hub=hub.Text.Trim(),code=code.Text.Trim(),ca_pem=Str(config,"ca_pem")});code.Clear();await Refresh(true);});
 LabelAt(panel,"采样间隔 / 秒",30,304);interval.Location=new Point(155,300);interval.Size=new Size(110,30);interval.Minimum=10;interval.Maximum=300;panel.Controls.Add(interval);
 Check(panel,processes,"上传进程",30,352);Check(panel,addresses,"上传网卡地址",235,352);Check(panel,hostname,"上传主机名",440,352);
 Check(panel,battery,"电池供电时暂停",30,394);Check(panel,startup,"登录 Windows 后自动运行",280,394);startup.Checked=Startup();
 save=ButtonAt(panel,"保存设置",30,449,185,async()=>{var next=new Dictionary<string,object>(config);next["interval"]=(int)interval.Value;next["processes"]=processes.Checked;next["addresses"]=addresses.Checked;next["hostname"]=hostname.Checked;next["pause_on_battery"]=battery.Checked;await Api("/api/local/config",next);SetStartup(startup.Checked);await Refresh(true);notice.Text="设置已保存";});
 pause=ButtonAt(panel,"暂停上传",235,449,185,async()=>{var next=new Dictionary<string,object>(config);next["paused"]=!paused;await Api("/api/local/config",next);await Refresh(true);});
 upload=ButtonAt(panel,"立即上传",440,449,195,async()=>{await Api("/api/local/upload",new{});notice.Text="已请求采样；网络退避期间不会绕过重试限制";});
 notice.Location=new Point(30,508);notice.Size=new Size(610,56);notice.BackColor=Color.Transparent;notice.ForeColor=Color.FromArgb(68,83,106);panel.Controls.Add(notice);
 LabelAt(panel,"关闭窗口后继续在托盘运行 · 凭据由 Windows 加密保护",30,580,610);
 client=new HttpClient(new HttpClientHandler{CookieContainer=new CookieContainer(),AllowAutoRedirect=false}){BaseAddress=new Uri("http://127.0.0.1:17860"),Timeout=TimeSpan.FromSeconds(20)};
 timer.Interval=5000;timer.Tick+=async(s,e)=>{if(!busy&&config!=null){busy=true;try{await Refresh(false);}catch{notice.Text="暂时无法读取采集器状态，保留现有设置";}finally{busy=false;}}};
 Shown+=async(s,e)=>{try{if(demo){password.Text="fictional-demo-password";await Login();return;}var p=Path.Combine(data,"native-password.dpapi");if(File.Exists(p)){password.Text=Encoding.UTF8.GetString(ProtectedData.Unprotect(File.ReadAllBytes(p),null,DataProtectionScope.CurrentUser));await Login();}}catch{notice.Text="请重新输入本机管理密码";}timer.Start();};
 FormClosed+=(s,e)=>{timer.Stop();timer.Dispose();client.Dispose();};EnabledControls(false);
 }
 void LabelAt(Control p,string text,int x,int y,int width=125){p.Controls.Add(new Label{Text=text,Location=new Point(x,y),Size=new Size(width,30),BackColor=Color.Transparent});}
 void Check(Control p,CheckBox c,string text,int x,int y){c.Text=text;c.Location=new Point(x,y);c.Size=new Size(210,32);c.BackColor=Color.Transparent;p.Controls.Add(c);}
 Button ButtonAt(Control p,string text,int x,int y,int w,Func<Task> action){var b=new Button{Text=text,Location=new Point(x,y),Size=new Size(w,40),FlatStyle=FlatStyle.Flat,BackColor=Color.FromArgb(246,249,255),ForeColor=Color.FromArgb(35,53,78)};b.FlatAppearance.BorderColor=Color.FromArgb(178,193,211);b.Click+=async(s,e)=>{if(busy)return;busy=true;b.Enabled=false;try{await action();}catch(Exception ex){notice.Text=ex.Message;}finally{busy=false;b.Enabled=true;}};p.Controls.Add(b);return b;}
 async Task<Dictionary<string,object>> Api(string path,object body=null){if(demo){await Task.Yield();if(path=="/api/session")return new Dictionary<string,object>{{"initialized",true}};if(path=="/api/login")return new Dictionary<string,object>{{"csrf","fictional"}};if(path=="/api/local/status")return json.Deserialize<Dictionary<string,object>>("{\"config\":{\"hub\":\"https://hub.example.com\",\"interval\":10},\"status\":{\"state\":\"online · 虚构演示\",\"successes\":128,\"sample\":{\"metrics\":{\"cpu\":{\"status\":\"supported\",\"data\":{\"percent\":12.4}},\"memory\":{\"status\":\"supported\",\"data\":{\"percent\":36.8}}}}}}");return new Dictionary<string,object>();}using(var request=new HttpRequestMessage(body==null?HttpMethod.Get:HttpMethod.Post,path)){if(body!=null){request.Content=new StringContent(json.Serialize(body),Encoding.UTF8,"application/json");request.Headers.Add("Origin","http://127.0.0.1:17860");if(csrf!="")request.Headers.Add("X-CSRF-Token",csrf);}using(var response=await client.SendAsync(request)){var raw=await response.Content.ReadAsStringAsync();if(!response.IsSuccessStatusCode){if(response.StatusCode==HttpStatusCode.Unauthorized){config=null;EnabledControls(false);}throw new Exception("操作未完成（HTTP "+(int)response.StatusCode+"），请检查密码、配对码或连接状态");}return json.Deserialize<Dictionary<string,object>>(raw);}}}
 async Task Login(){var session=await Api("/api/session");var path=Flag(session,"initialized")?"/api/login":"/api/setup";string bootstrap="";if(path=="/api/setup")bootstrap=File.ReadAllText(Path.Combine(data,"bootstrap-secret")).Trim();var result=await Api(path,new{password=password.Text,bootstrap=bootstrap});csrf=Str(result,"csrf");if(!demo){Directory.CreateDirectory(data);File.WriteAllBytes(Path.Combine(data,"native-password.dpapi"),ProtectedData.Protect(Encoding.UTF8.GetBytes(password.Text),null,DataProtectionScope.CurrentUser));}password.Clear();login.Text="重新解锁";await Refresh(true);notice.Text="已解锁，本机设置通过原生窗口管理";}
 async Task Refresh(bool fields){var result=await Api("/api/local/status");config=Dict(result["config"]);var state=Dict(result["status"]);paused=Flag(config,"paused");status.Text="状态："+Str(state,"state")+"  ·  成功上报 "+Str(state,"successes")+" 次";pause.Text=paused?"恢复上传":"暂停上传";var sample=Dict(state.ContainsKey("sample")?state["sample"]:null);var metric=Dict(sample.ContainsKey("metrics")?sample["metrics"]:null);metrics.Text="CPU "+Percent(metric,"cpu")+"     内存 "+Percent(metric,"memory");if(fields){hub.Text=Str(config,"hub");int seconds;interval.Value=int.TryParse(Str(config,"interval"),out seconds)?Math.Max(10,Math.Min(300,seconds)):10;processes.Checked=Flag(config,"processes");addresses.Checked=Flag(config,"addresses");hostname.Checked=Flag(config,"hostname");battery.Checked=Flag(config,"pause_on_battery");}EnabledControls(true);}
 string Percent(Dictionary<string,object> m,string name){if(!m.ContainsKey(name))return "—";var block=Dict(m[name]);if(Str(block,"status")!="supported")return "—";var d=Dict(block.ContainsKey("data")?block["data"]:null);double value;return double.TryParse(Str(d,"percent"),out value)?value.ToString("F1")+"%":"—";}
 void EnabledControls(bool value){save.Enabled=pair.Enabled=pause.Enabled=upload.Enabled=value;}
 bool Startup(){using(var key=Registry.CurrentUser.OpenSubKey(@"Software\Microsoft\Windows\CurrentVersion\Run"))return key!=null&&key.GetValue("ChronosAgent")!=null;}
 void SetStartup(bool value){using(var key=Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Run")){if(value)key.SetValue("ChronosAgent","\""+Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"chronos-agent.exe")+"\"");else key.DeleteValue("ChronosAgent",false);}}
}
