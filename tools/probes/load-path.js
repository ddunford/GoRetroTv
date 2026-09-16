function w(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function h(v){ return '0x'+(v>>>0).toString(16).padStart(8,'0'); }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }
function poke16(a,v){ window.__poke(a, [(v>>>8)&255, v&255]); }
function pokeStr(a,s){ var b=[]; for(var i=0;i<s.length;i++) b.push(s.charCodeAt(i)&0x7F); b.push(0); window.__poke(a,b); }
await new Promise(r=>setTimeout(r,30000));
window.__poke(0x80105D14, [0xFF,0xFF,0xFF,0xFF]);
var table = w(0x80106F54), ID = 3, MODULE = 1;
var REC = 0x803F0000, MODTAB = 0x803F1000, LIST = 0x803F3000;
var A = MODTAB + 0x40, B = MODTAB + 0x70;

for (var i = 0; i < 0x40; i += 4) poke32(REC + i, 0);
window.__poke(REC + 0x04, [(ID>>>8)&255, ID&255]);
window.__poke(REC + 0x08, [0x43,0x52,0x53,0x4C]);
poke32(REC + 0x1C, 1);
poke32(REC + 0x2C, MODTAB);

// THE MODULE TABLE, now with the fields 0x8008ED54 intersects. Sub-structure A is reached by a
// 16-bit SELF-RELATIVE offset at +0x00 and mirrors the table's own shape (+0x04/+0x08/+0x0C/
// +0x10 dwords, then a payload at +0x14); B likewise from +0x02. Two of the six words the
// capability set carries are handed to 0x8007E2C4, which is the string setter this image also
// uses for "resident" and the modem's APN -- so A+0x14 and B are NAMES, not numbers.
for (var j = 0; j < 0x100; j += 4) poke32(MODTAB + j, 0);
poke16(MODTAB + 0x00, 0x40);          // -> A
poke16(MODTAB + 0x02, 0x70);          // -> B
poke32(MODTAB + 0x04, 2);
poke32(MODTAB + 0x08, 0xFFFFFFFF);    // limit: let the min pick A's
poke32(MODTAB + 0x0C, 0xFFFFFFFF);    // permission mask: let the AND pick A's
poke32(MODTAB + 0x10, 0xFFFFFFFF);
poke32(MODTAB + 0x18, 1);             // module count
poke32(MODTAB + 0x28, 0x0000000F);    // module entry 1
poke32(A + 0x04, 2);                  // capability[2] -- 0x8007B170 skips on 0 and on 1
poke32(A + 0x08, 0x00010000);
poke32(A + 0x0C, 0xFFFFFFFF);
poke32(A + 0x10, 0x00010000);
pokeStr(A + 0x14, "GUIDE");
pokeStr(B, "OpenTV");

for (var q = 0; q < 0x60; q += 4) poke32(LIST + q, 0);
poke32(0x801064F8, LIST);
var e = LIST + 8;
poke32(e + 0x00, (ID << 16) | MODULE);
poke32(e + 0x04, 0x803F2000);
poke32(e + 0x08, 0x00001000);
poke32(e + 0x0C, 0x803F2000);
poke32(e + 0x10, 0);                  // status: any non-zero is failure at this site
poke32(table + (((ID - 1) % 256) * 4) + 4, REC);
poke32(0x80106520, (ID << 16) | MODULE);

window.__traceCalls([
  { pc: 0x8007D06C, name: 'dbg', fmt: true },
  { pc: 0x8005141C, name: 'autoload', args: 1 },
  { pc: 0x80051B2C, name: 'intprtMsg', args: 1 },
  { pc: 0x8008ED54, name: 'securityGate', args: 1 },
  { pc: 0x8007AFA4, name: 'useCapabilities', args: 1 },
  { pc: 0x8007B170, name: 'f_8007B170', args: 1 },
  { pc: 0x8007E2C4, name: 'setName', args: 2 },
  { pc: 0x80036D0C, name: 'appStart', args: 2 }
]);
window.__traceClear();
var r = window.__call(0x8005141C, 1, 0, 0, 0, 0);
await new Promise(rr=>setTimeout(rr,6000));
function surf(){
  var b = parseInt(document.getElementById('fb-base').value,16) || 0;
  if(!b) return 'none';
  var bytes = window.__peek(b, 288*360), s2 = 2166136261;
  for(var i2=0;i2<bytes.length;i2++) s2 = (Math.imul(s2 ^ bytes[i2], 16777619))>>>0;
  return h(s2);
}
return { call: { ok: r.ok, why: r.why, v0: r.v0 },
         after: { intprtState: h(w(0x80106508)), moduleHandle: h(w(0x80106520)),
                  appId: h(w(0x80106524)), blits: window.__blitLog().length,
                  surface: surf(), tasks2: window.__tasks().n,
                  task30: window.__tasks().tasks.filter(function(t){return t.name==='TASK30';})
                            .map(function(t){ return 'st='+t.st+' runs='+t.runs; })[0] },
         trace: window.__traceLog().map(function(e2){
           return e2.name + '(' + (e2.text !== undefined ? e2.text : e2.a.join(', ')) + ')'; }),
         tasks: window.__tasks().n };
