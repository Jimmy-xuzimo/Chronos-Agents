using System;
using System.IO;
using System.Collections.Generic;
using System.Web.Script.Serialization;
using System.Linq;
using System.Drawing.Text;
static class DesktopDesign {
 static readonly Dictionary<string,object> Data=Load();
 static Dictionary<string,object> Load(){using(var s=typeof(DesktopDesign).Assembly.GetManifestResourceStream("Chronos.DesktopDesign.json")){if(s==null)throw new InvalidOperationException("Missing design tokens");using(var r=new StreamReader(s))return new JavaScriptSerializer().Deserialize<Dictionary<string,object>>(r.ReadToEnd());}}
 public static string Value(string name){return Convert.ToString(Data[name]);}
 public static string Gradient(int index){return Convert.ToString(((System.Collections.IEnumerable)Data["gradient"]).Cast<object>().ElementAt(index));}
 public static readonly string FontName=ChooseFont();
 static string ChooseFont(){using(var fonts=new InstalledFontCollection()){var available=fonts.Families.Select(f=>f.Name).ToArray();foreach(var name in (System.Collections.IEnumerable)Data["font"]){var text=Convert.ToString(name);if(available.Contains(text,StringComparer.OrdinalIgnoreCase))return text;}return "Segoe UI";}}
}
