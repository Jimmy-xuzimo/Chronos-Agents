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
 Text="Chronos Agent · 设备采集器"+(demo?" · 虚构演示":"");ClientSize=new Size(580,510);MinimumSize=new Size(540,540);MaximizeBox=false;StartPosition=FormStartPosition.CenterScreen;AutoScaleMode=AutoScaleMode.Dpi;Font=new Font("Microsoft YaHei UI",9);BackColor=SystemColors.Control;try{Icon=Icon.ExtractAssociatedIcon(Application.ExecutablePath);}catch{}
 var panel=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(18),ColumnCount=1,RowCount=12};panel.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));Controls.Add(panel);
 panel.Controls.Add(new Label{Text="Chronos Agent",Font=new Font(Font.FontFamily,17,FontStyle.Bold),AutoSize=true,Margin=new Padding(0,0,0,6)});
 status.AutoSize=true;status.Text="本机采集 · 主动上报";panel.Controls.Add(status);metrics.AutoSize=true;metrics.Margin=new Padding(0,4,0,8);panel.Controls.Add(metrics);
 var form=new TableLayoutPanel{Dock=DockStyle.Top,AutoSize=true,ColumnCount=2};form.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));form.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));panel.Controls.Add(form);
 password.UseSystemPasswordChar=true;password.Width=260;login=MakeButton("解锁",async()=>await Login());AddField(form,"管理密码",Flow(password,login));
 hub.Dock=DockStyle.Fill;AddField(form,"接收中心",hub);
 code.Width=260;pair=MakeButton("配对设备",async()=>{await Api("/api/local/pair",new{hub=hub.Text.Trim(),code=code.Text.Trim(),ca_pem=Str(config,"ca_pem")});code.Clear();await Refresh(true);});AddField(form,"配对码",Flow(code,pair));
 interval.Minimum=10;interval.Maximum=300;interval.Width=90;AddField(form,"间隔 / 秒",interval);
 SetCheck(processes,"上传进程");SetCheck(addresses,"上传网卡地址");SetCheck(hostname,"上传主机名");panel.Controls.Add(Flow(processes,addresses,hostname));
 SetCheck(battery,"电池供电时暂停");SetCheck(startup,"登录 Windows 后自动运行");startup.Checked=Startup();panel.Controls.Add(Flow(battery,startup));
 save=MakeButton("保存设置",async()=>{var next=new Dictionary<string,object>(config);next["interval"]=(int)interval.Value;next["processes"]=processes.Checked;next["addresses"]=addresses.Checked;next["hostname"]=hostname.Checked;next["pause_on_battery"]=battery.Checked;await Api("/api/local/config",next);if(!demo)SetStartup(startup.Checked);await Refresh(true);notice.Text="设置已保存";});
 pause=MakeButton("暂停上传",async()=>{var next=new Dictionary<string,object>(config);next["paused"]=!paused;await Api("/api/local/config",next);await Refresh(true);});
 upload=MakeButton("立即上传",async()=>{await Api("/api/local/upload",new{});notice.Text="已请求采样；重试退避仍然生效";});panel.Controls.Add(Flow(save,pause,upload));
 notice.AutoSize=true;notice.MaximumSize=new Size(510,0);notice.ForeColor=SystemColors.GrayText;notice.Margin=new Padding(0,8,0,4);panel.Controls.Add(notice);
 panel.Controls.Add(new Label{Text="关闭窗口后在托盘继续运行 · 本机密码由 Windows 加密保存",AutoSize=true,ForeColor=SystemColors.GrayText});
 client=new HttpClient(new HttpClientHandler{CookieContainer=new CookieContainer(),AllowAutoRedirect=false}){BaseAddress=new Uri("http://127.0.0.1:17860"),Timeout=TimeSpan.FromSeconds(20)};
 timer.Interval=5000;timer.Tick+=async(s,e)=>{if(!busy&&config!=null){busy=true;try{await Refresh(false);}catch{notice.Text="暂时无法读取采集器状态，保留现有设置";}finally{busy=false;}}};
 Shown+=async(s,e)=>{try{if(demo){password.Text="fictional-demo-password";await Login();return;}var p=Path.Combine(data,"native-password.dpapi");if(File.Exists(p)){password.Text=Encoding.UTF8.GetString(ProtectedData.Unprotect(File.ReadAllBytes(p),null,DataProtectionScope.CurrentUser));await Login();}}catch{notice.Text="请重新输入本机管理密码";}timer.Start();};
 FormClosed+=(s,e)=>{timer.Stop();timer.Dispose();client.Dispose();};EnabledControls(false);
 }
 FlowLayoutPanel Flow(params Control[] controls){var flow=new FlowLayoutPanel{AutoSize=true,Dock=DockStyle.Top,WrapContents=true,Margin=new Padding(0,3,0,3)};foreach(var c in controls){c.Margin=new Padding(0,2,12,2);flow.Controls.Add(c);}return flow;}
 void SetCheck(CheckBox c,string text){c.Text=text;c.AutoSize=true;}
 void AddField(TableLayoutPanel table,string text,Control control){int row=table.RowCount++;var label=new Label{Text=text,AutoSize=true,Anchor=AnchorStyles.Left,Margin=new Padding(0,6,14,6)};control.Margin=new Padding(0,4,0,4);table.Controls.Add(label,0,row);table.Controls.Add(control,1,row);}
 Button MakeButton(string text,Func<Task> action){var b=new Button{Text=text,AutoSize=true,Padding=new Padding(8,4,8,4),UseVisualStyleBackColor=true};b.Click+=async(s,e)=>{if(busy)return;busy=true;b.Enabled=false;try{await action();}catch(Exception ex){notice.Text=ex.Message;}finally{busy=false;b.Enabled=true;}};return b;}
 async Task<Dictionary<string,object>> Api(string path,object body=null){if(demo){await Task.Yield();if(path=="/api/session")return new Dictionary<string,object>{{"initialized",true}};if(path=="/api/login")return new Dictionary<string,object>{{"csrf","fictional"}};if(path=="/api/local/status")return json.Deserialize<Dictionary<string,object>>("{\"config\":{\"hub\":\"https://hub.example.com\",\"interval\":10},\"status\":{\"state\":\"online · 虚构演示\",\"successes\":128,\"sample\":{\"metrics\":{\"cpu\":{\"status\":\"supported\",\"data\":{\"percent\":12.4}},\"memory\":{\"status\":\"supported\",\"data\":{\"percent\":36.8}}}}}}");return new Dictionary<string,object>();}using(var request=new HttpRequestMessage(body==null?HttpMethod.Get:HttpMethod.Post,path)){if(body!=null){request.Content=new StringContent(json.Serialize(body),Encoding.UTF8,"application/json");request.Headers.Add("Origin","http://127.0.0.1:17860");if(csrf!="")request.Headers.Add("X-CSRF-Token",csrf);}using(var response=await client.SendAsync(request)){var raw=await response.Content.ReadAsStringAsync();if(!response.IsSuccessStatusCode){if(response.StatusCode==HttpStatusCode.Unauthorized){config=null;EnabledControls(false);}throw new Exception("操作未完成（HTTP "+(int)response.StatusCode+"），请检查密码、配对码或连接状态");}return json.Deserialize<Dictionary<string,object>>(raw);}}}
 async Task Login(){var session=await Api("/api/session");var path=Flag(session,"initialized")?"/api/login":"/api/setup";string bootstrap="";if(path=="/api/setup")bootstrap=File.ReadAllText(Path.Combine(data,"bootstrap-secret")).Trim();var result=await Api(path,new{password=password.Text,bootstrap=bootstrap});csrf=Str(result,"csrf");if(!demo){Directory.CreateDirectory(data);File.WriteAllBytes(Path.Combine(data,"native-password.dpapi"),ProtectedData.Protect(Encoding.UTF8.GetBytes(password.Text),null,DataProtectionScope.CurrentUser));}password.Clear();login.Text="重新解锁";await Refresh(true);notice.Text="已解锁，本机设置通过原生窗口管理";}
 async Task Refresh(bool fields){var result=await Api("/api/local/status");config=Dict(result["config"]);var state=Dict(result["status"]);paused=Flag(config,"paused");status.Text="状态："+Str(state,"state")+"  ·  成功上报 "+Str(state,"successes")+" 次";pause.Text=paused?"恢复上传":"暂停上传";var sample=Dict(state.ContainsKey("sample")?state["sample"]:null);var metric=Dict(sample.ContainsKey("metrics")?sample["metrics"]:null);metrics.Text="CPU "+Percent(metric,"cpu")+"     内存 "+Percent(metric,"memory");if(fields){hub.Text=Str(config,"hub");int seconds;interval.Value=int.TryParse(Str(config,"interval"),out seconds)?Math.Max(10,Math.Min(300,seconds)):10;processes.Checked=Flag(config,"processes");addresses.Checked=Flag(config,"addresses");hostname.Checked=Flag(config,"hostname");battery.Checked=Flag(config,"pause_on_battery");}EnabledControls(true);}
 string Percent(Dictionary<string,object> m,string name){if(!m.ContainsKey(name))return "—";var block=Dict(m[name]);if(Str(block,"status")!="supported")return "—";var d=Dict(block.ContainsKey("data")?block["data"]:null);double value;return double.TryParse(Str(d,"percent"),out value)?value.ToString("F1")+"%":"—";}
 void EnabledControls(bool value){save.Enabled=pair.Enabled=pause.Enabled=upload.Enabled=value;}
 bool Startup(){using(var key=Registry.CurrentUser.OpenSubKey(@"Software\Microsoft\Windows\CurrentVersion\Run"))return key!=null&&key.GetValue("ChronosAgent")!=null;}
 void SetStartup(bool value){using(var key=Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Run")){if(value)key.SetValue("ChronosAgent","\""+Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"chronos-agent.exe")+"\"");else key.DeleteValue("ChronosAgent",false);}}
}
