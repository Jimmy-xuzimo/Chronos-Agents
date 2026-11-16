using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Windows.Forms;

sealed class GlassButton : Button {
 bool hover;
 public bool Primary;
 public GlassButton(){SetStyle(ControlStyles.UserPaint|ControlStyles.AllPaintingInWmPaint|ControlStyles.OptimizedDoubleBuffer,true);FlatStyle=FlatStyle.Flat;FlatAppearance.BorderSize=0;MinimumSize=new Size(0,Convert.ToInt32(DesktopDesign.Value("buttonHeight")));Cursor=Cursors.Hand;}
 protected override void OnMouseEnter(EventArgs e){hover=true;Invalidate();base.OnMouseEnter(e);}
 protected override void OnMouseLeave(EventArgs e){hover=false;Invalidate();base.OnMouseLeave(e);}
 protected override void OnPaint(PaintEventArgs e){
  e.Graphics.SmoothingMode=SmoothingMode.AntiAlias;
  var rect=new Rectangle(1,1,Width-3,Height-3);if(rect.Width<5||rect.Height<5)return;
  Color accent=ColorTranslator.FromHtml(DesktopDesign.Value("accent"));
  Color fill=Primary?accent:Color.White;
  if(hover)fill=AgentSkin.Blend(fill,accent,.12f);
  if(!Enabled)fill=AgentSkin.Blend(fill,Color.White,.5f);
  using(var path=AgentSkin.Round(rect,Convert.ToInt32(DesktopDesign.Value("controlRadius"))))using(var brush=new SolidBrush(fill))using(var pen=new Pen(Enabled?ColorTranslator.FromHtml(DesktopDesign.Value("line")):Color.LightGray)){e.Graphics.FillPath(brush,path);e.Graphics.DrawPath(pen,path);}
  TextRenderer.DrawText(e.Graphics,Text,Font,rect,Enabled?(Primary?Color.White:ColorTranslator.FromHtml(DesktopDesign.Value("ink"))):Color.Gray,TextFormatFlags.HorizontalCenter|TextFormatFlags.VerticalCenter|TextFormatFlags.SingleLine|TextFormatFlags.EndEllipsis);
  if(Focused&&ShowFocusCues)ControlPaint.DrawFocusRectangle(e.Graphics,new Rectangle(5,5,Math.Max(0,Width-10),Math.Max(0,Height-10)));
 }
}
