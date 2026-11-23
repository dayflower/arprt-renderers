package main

import (
	"bytes"
	"errors"
	"html/template"
)

// Colors are validated #rrggbb values before they reach the template.
var page = template.Must(template.New("swatch").Funcs(template.FuncMap{
	"css": func(s string) template.CSS { return template.CSS(s) },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Swatch</title><style>
*{box-sizing:border-box}html,body{margin:0;background:#fff;color:#000}
body{width:100vw;height:100vh;padding:16px;display:flex;flex-direction:column;gap:8px;font-family:"Noto Sans CJK JP","Noto Sans JP",Arial,sans-serif;overflow:hidden}
header{display:flex;align-items:baseline;gap:12px;padding-bottom:6px;border-bottom:2px solid #000;flex-shrink:0;min-width:0;white-space:nowrap}
h1{margin:0;font-size:20px;line-height:1.2;font-weight:750;flex-shrink:0}
.meta{font-size:14px;line-height:1.2;font-weight:600;flex-shrink:0}
.note{font-size:14px;line-height:1.2;min-width:0;overflow:hidden;text-overflow:ellipsis}
#grid{flex:1;min-height:0;display:grid;grid-template-columns:repeat({{.Columns}},minmax(0,1fr));grid-template-rows:repeat({{.Rows}},minmax(0,1fr));gap:4px}
.tile{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:2px;padding:2px 4px;min-width:0;min-height:0;overflow:hidden}
.tile.outline{border:1px solid #000}
.hex{font-size:18px;line-height:1.15;font-weight:700;white-space:nowrap;font-variant-numeric:tabular-nums}
.label{font-size:12px;line-height:1.2;max-width:100%;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
</style></head><body>
<header><h1>SWATCH</h1><span class="meta">{{.Palette}}</span><span class="meta">rotation {{.Rotation}}</span>{{if .Note}}<span class="note" title="{{.Note}}">{{.Note}}</span>{{end}}</header>
<main id="grid">{{range .Slots}}{{with .Tile}}<div class="tile{{if .Outline}} outline{{end}}" data-color="{{.Color}}" style="background:{{css .Color}};color:{{css .Text}}"><span class="hex">{{.Color}}</span>{{if .Label}}<span class="label" title="{{.Label}}">{{.Label}}</span>{{end}}</div>{{else}}<div class="slot"></div>{{end}}{{end}}</main>
<script>
(async()=>{
try{
 await document.fonts.ready;
 const body=document.body.getBoundingClientRect(),style=getComputedStyle(document.body);
 const left=body.left+parseFloat(style.paddingLeft),top=body.top+parseFloat(style.paddingTop),right=body.right-parseFloat(style.paddingRight),bottom=body.bottom-parseFloat(style.paddingBottom);
 const inside=box=>box.width>0&&box.height>0&&box.left>=left-.5&&box.top>=top-.5&&box.right<=right+.5&&box.bottom<=bottom+.5;
 for(const element of document.querySelectorAll('header,h1,.meta,#grid,#grid>*')){
   if(!inside(element.getBoundingClientRect()))throw Error('content outside safe layout');
 }
 for(const element of document.querySelectorAll('h1,.meta'))if(element.scrollWidth>element.clientWidth+1)throw Error('header overflow');
 for(const element of document.querySelectorAll('#grid>*')){
   const box=element.getBoundingClientRect();
   if(box.width<64||box.height<36)throw Error('tiles too small');
 }
 for(const hex of document.querySelectorAll('.hex')){
   const tile=hex.parentElement.getBoundingClientRect(),range=document.createRange();range.selectNodeContents(hex);
   const ink=range.getBoundingClientRect();
   if(ink.left<tile.left||ink.right>tile.right||ink.top<tile.top||ink.bottom>tile.bottom)throw Error('color text overflow');
 }
 await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 document.body.id='swatch-ready';
}catch(error){document.documentElement.dataset.layoutError=error.message;}
})();
</script></body></html>`))

func renderHTML(v view) ([]byte, error) {
	var b bytes.Buffer
	if err := page.Execute(&b, v); err != nil || b.Len() > htmlLimit {
		return nil, errors.New("cannot render swatch HTML")
	}
	return b.Bytes(), nil
}
