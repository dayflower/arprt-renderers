package main

import (
	"bytes"
	"errors"
	"html/template"
)

var page = template.Must(template.New("countdown").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Countdown</title><style>
*{box-sizing:border-box}html,body{margin:0;background:#fff;color:#000}
body{width:100vw;height:100vh;padding:24px;display:flex;flex-direction:column;gap:16px;font-family:"Noto Sans CJK JP","Noto Sans JP",Arial,sans-serif;overflow:hidden}
header{display:flex;align-items:center;gap:14px;padding-bottom:12px;border-bottom:2px solid #000;flex-shrink:0}
header svg{width:36px;height:36px;flex-shrink:0}h1{margin:0;font-size:32px;line-height:1.15;font-weight:750;white-space:nowrap}
main{flex:1;min-height:0;display:flex;align-items:center;justify-content:center;text-align:center}
#group{width:100%;display:flex;flex-direction:column;align-items:center;gap:12px;flex-shrink:0}
.title{font-size:28px;line-height:1.5;font-weight:650;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;width:100%;flex-shrink:0}
#count{font-size:220px;font-weight:800;line-height:normal;letter-spacing:-.035em;white-space:nowrap;font-variant-numeric:lining-nums tabular-nums}
#status{font-size:20px;font-weight:600;line-height:1.2;letter-spacing:.12em;white-space:nowrap}
footer{display:flex;justify-content:space-between;gap:24px;border-top:2px solid #000;padding-top:12px;flex-shrink:0}
.date-group{display:flex;flex-direction:column;gap:4px}.date-group:last-child{text-align:right}
.label{font-size:16px;font-weight:650;line-height:1.2}.date{font-size:20px;font-weight:700;line-height:1.2;white-space:nowrap;font-variant-numeric:tabular-nums}
@media (orientation:portrait){footer{flex-direction:column;gap:16px}.date-group:last-child{text-align:left}#count{font-size:200px}}
</style></head><body>
<header><svg viewBox="0 0 36 36" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true"><rect x="4" y="7" width="28" height="26" rx="3"/><path d="M11 2v10M25 2v10M4 16h28M11 21h3M22 21h3M11 27h3M22 27h3"/></svg><h1>COUNTDOWN</h1></header>
<main><div id="group">{{if .Title}}<div class="title" title="{{.Title}}">{{.Title}}</div>{{end}}<div id="count">{{.Count}}</div><div id="status">{{.Status}}</div></div></main>
<footer><div class="date-group"><span class="label">TARGET</span><time class="date" datetime="{{.Target}}">{{.Target}}</time></div><div class="date-group"><span class="label">TODAY</span><time class="date" datetime="{{.Today}}">{{.Today}}</time></div></footer>
<script>
(async()=>{
try{
 await document.fonts.ready;
 const main=document.querySelector('main'),group=document.getElementById('group'),count=document.getElementById('count');
 const fits=()=>group.getBoundingClientRect().height<=main.clientHeight+.1&&count.getBoundingClientRect().width<=main.clientWidth+.1;
 let size=innerHeight>innerWidth?200:220;
 while(size>=64){count.style.fontSize=size+'px';if(fits())break;size-=2;}
 if(size<64)throw Error('count does not fit');
 const body=document.body.getBoundingClientRect(),style=getComputedStyle(document.body);
 const left=body.left+parseFloat(style.paddingLeft),top=body.top+parseFloat(style.paddingTop),right=body.right-parseFloat(style.paddingRight),bottom=body.bottom-parseFloat(style.paddingBottom);
 for(const element of document.querySelectorAll('header,header svg,h1,main,#group,#count,#status,.title,footer,.date-group,.label,.date')){
   const box=element.getBoundingClientRect();
   if(box.width<=0||box.height<=0||box.left<left-.5||box.top<top-.5||box.right>right+.5||box.bottom>bottom+.5)throw Error('content outside safe layout');
   if(!element.classList.contains('title')&&element.scrollWidth>element.clientWidth+1)throw Error('required text overflow');
 }
 const range=document.createRange();range.selectNodeContents(count);
 const ink=range.getBoundingClientRect(),box=count.getBoundingClientRect();
 if(ink.left<left-.5||ink.right>right+.5||ink.top<box.top-1||ink.bottom>box.bottom+1)throw Error('number overflow');
 await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 document.body.id='countdown-ready';
}catch(error){document.documentElement.dataset.layoutError=error.message;}
})();
</script></body></html>`))

func renderHTML(view countdownView) ([]byte, error) {
	var b bytes.Buffer
	if err := page.Execute(&b, view); err != nil || b.Len() > htmlLimit {
		return nil, errors.New("cannot render countdown HTML")
	}
	return b.Bytes(), nil
}
