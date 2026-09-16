// A TIMELINE CENSUS with the DRAWING watched alongside it.
//
// Two runs of the short census disagreed about which natives a key press runs, which is what a
// periodic burst sampled against a short window looks like -- so this samples 2-second buckets for
// a minute with NO key, then presses one. That is how (1,0xC7) x100 and (1,0x2D) x49 turned out to
// run in the first six seconds of an untouched box rather than in response to anything.
//
// It also watches the blit counter and the visible plane, which is how the ordering shows up at all:
// the box boots, sits black, and only draws the blue screen about a minute later -- blits 4 -> 8 in
// the t=22 bucket, alongside the only (1,0xD2) and (1,0xE4) calls outside a key press.
var NAT = [
  [0,0x80081B00],
  [1,0x80081B10],
  [2,0x80081B20],
  [3,0x80081B30],
  [4,0x800711EC],
  [5,0x80071214],
  [6,0x8007123C],
  [7,0x80071274],
  [8,0x800712BC],
  [9,0x800712CC],
  [10,0x800712FC],
  [11,0x8007132C],
  [12,0x80071360],
  [13,0x80071460],
  [14,0x80071420],
  [15,0x80071440],
  [16,0x800714B0],
  [17,0x800714C0],
  [18,0x80071520],
  [19,0x800714D0],
  [20,0x800714E4],
  [21,0x800713C0],
  [22,0x80071530],
  [23,0x80071488],
  [24,0x800820E0],
  [25,0x800820C0],
  [26,0x800820D0],
  [27,0x800820F0],
  [28,0x80082100],
  [29,0x80082110],
  [30,0x80081C28],
  [31,0x80081C38],
  [32,0x80081C48],
  [33,0x80081C68],
  [34,0x80081CA8],
  [35,0x80081CB8],
  [36,0x80081CC8],
  [37,0x80081CD8],
  [38,0x80081CE8],
  [39,0x80081D10],
  [40,0x80081D20],
  [41,0x80081D40],
  [42,0x80081C88],
  [43,0x80050F40],
  [44,0x80050F50],
  [45,0x80082040],
  [46,0x80082050],
  [47,0x80081DD0],
  [48,0x80081DE0],
  [49,0x80081DF0],
  [50,0x80081DB0],
  [51,0x80081DC0],
  [52,0x80081D80],
  [53,0x80081D60],
  [54,0x80081D70],
  [55,0x80081F10],
  [56,0x80081F20],
  [57,0x80081F44],
  [58,0x80081F34],
  [59,0x80082020],
  [60,0x80081D90],
  [61,0x80081DA0],
  [62,0x80082124],
  [63,0x8008213C],
  [64,0x80082150],
  [65,0x80082160],
  [66,0x80082170],
  [67,0x80081E00],
  [68,0x80081E20],
  [69,0x80081E30],
  [70,0x80081E40],
  [71,0x80081E50],
  [72,0x80081E60],
  [73,0x80081E70],
  [74,0x80081E80],
  [75,0x80081E90],
  [76,0x80081EA0],
  [77,0x80081EB0],
  [78,0x80081ED0],
  [79,0x80081EE0],
  [80,0x80081F00],
  [81,0x80081EF0],
  [82,0x80082190],
  [83,0x800821A0],
  [84,0x800821B0],
  [85,0x80081E10],
  [86,0x800821C0],
  [87,0x80081570],
  [88,0x80081590],
  [89,0x80081580],
  [90,0x80081540],
  [91,0x80081550],
  [92,0x8008153C],
  [93,0x800816E4],
  [94,0x80081714],
  [95,0x80081560],
  [96,0x800816F4],
  [97,0x80081704],
  [98,0x80081640],
  [99,0x80081650],
  [100,0x800815B0],
  [101,0x800815C0],
  [102,0x80081610],
  [103,0x80081620],
  [104,0x80081684],
  [105,0x80081694],
  [106,0x800816B4],
  [107,0x800816C4],
  [108,0x800815E0],
  [109,0x800815F0],
  [110,0x80081654],
  [111,0x80081664],
  [112,0x80081724],
  [113,0x80081734],
  [114,0x80081744],
  [115,0x80081754],
  [116,0x80081764],
  [117,0x800815A0],
  [118,0x80081794],
  [119,0x800817A4],
  [120,0x800817C4],
  [121,0x80081774],
  [122,0x800817B4],
  [123,0x800817D4],
  [124,0x80081784],
  [125,0x800817E4],
  [126,0x80081804],
  [127,0x800817F4],
  [128,0x80081814],
  [129,0x80081824],
  [130,0x80081844],
  [131,0x80081834],
  [132,0x80081864],
  [133,0x80081874],
  [134,0x80081950],
  [135,0x80081960],
  [136,0x80081980],
  [137,0x80081990],
  [138,0x800819B0],
  [139,0x800819C0],
  [140,0x80081930],
  [141,0x80081940],
  [142,0x80081A10],
  [143,0x80081A30],
  [144,0x80081A40],
  [145,0x80081A50],
  [146,0x80081A20],
  [147,0x80081A70],
  [148,0x80081A90],
  [149,0x80081AB0],
  [150,0x80081A80],
  [151,0x80081AA0],
  [152,0x80081AC0],
  [153,0x80081AD0],
  [154,0x80081AE0],
  [155,0x80081AF0],
  [156,0x80082060],
  [157,0x80082080],
  [158,0x80082070],
  [159,0x800865FC],
  [160,0x800865EC],
  [161,0x8008660C],
  [162,0x8008663C],
  [163,0x8008661C],
  [164,0x8008662C],
  [165,0x8008664C],
  [166,0x8009CDC4],
  [167,0x8009CDD4],
  [168,0x8009CDE4],
  [169,0x80085F04],
  [170,0x80085F14],
  [171,0x80081244],
  [172,0x8008125C],
  [173,0x80081274],
  [174,0x80081218],
  [175,0x80081298],
  [176,0x800812BC],
  [177,0x800812E0],
  [178,0x80081350],
  [179,0x80081450],
  [180,0x80081438],
  [181,0x800812F8],
  [182,0x80081388],
  [183,0x800813E0],
  [184,0x80081488],
  [185,0x800814C8],
  [186,0x80082180],
  [187,0x8009CCB4],
  [188,0x8009CCCC],
  [189,0x8009CD00],
  [190,0x8009CD2C],
  [191,0x8009CD40],
  [192,0x80086414],
  [193,0x80086424],
  [194,0x80086434],
  [195,0x80071500],
  [196,0x80071510],
  [197,0x800819E0],
  [198,0x80081A00],
  [199,0x80081B40],
  [200,0x80081B50],
  [201,0x80081B60],
  [202,0x80081B70],
  [203,0x80081B80],
  [204,0x80081B90],
  [205,0x80071630],
  [206,0x80071650],
  [207,0x80071558],
  [208,0x80071590],
  [209,0x80071610],
  [210,0x80082090],
  [211,0x800820B0],
  [212,0x800820A0],
  [213,0x8007138C],
  [214,0x800713EC],
  [215,0x80071430],
  [216,0x80071450],
  [217,0x80071578],
  [218,0x800715E0],
  [219,0x80071534],
  [220,0x80071548],
  [221,0x800715C0],
  [222,0x80081D50],
  [223,0x800715A0],
  [224,0x80081C78],
  [225,0x800715D0],
  [226,0x800715B0],
  [227,0x80081D30],
  [228,0x80081C58],
  [229,0x80081C98],
  [230,0x800715F0],
  [231,0x80071600],
  [232,0x80081EC0],
  [233,0x80071620],
  [234,0x80071640],
  [235,0x80081CF8]
];
// __pcHits KEYS ITS RESULT WITH THE PAGE'S hex32, WHICH UPPERCASES. A lower-case lookup misses
// every address containing a hex letter and returns a perfectly plausible ZERO -- this census
// reported (1,0xE4) and (1,0xC7) as never called for exactly that reason, while natives whose shim
// address happens to be all digits counted correctly. So the key is built the page's way, and a
// missing key is a HARNESS failure rather than a count of zero.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function hitOf(hits, addr){
  var k = h(addr);
  if (!(k in hits)) throw new Error('pcHits has no key ' + k + ' -- the lookup is not matching');
  return hits[k];
}
function ic(){ return parseInt(document.getElementById('icount').textContent.replace(/,/g,''),10); }
window.__profile(true);
var ARGS = NAT.map(function(r){ return r[1]; });
function snap(){
  var hits = window.__pcHits.apply(null, ARGS), o = {};
  NAT.forEach(function(r){ o[r[0]] = hitOf(hits, r[1]); });
  return o;
}
function delta(a,b){
  var s = [];
  NAT.forEach(function(r){ var d = b[r[0]]-a[r[0]]; if (d>0) s.push('0x'+r[0].toString(16).toUpperCase().padStart(2,'0')+'x'+d); });
  return s;
}
// The visible plane, hashed, so "the screen changed" is a measurement and not an impression.
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,4);
  return { hash: h(s), top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}
var buckets = [], prev = snap(), t0 = ic(), nb = window.__blitLog().length;
async function tick(t, tag){
  await new Promise(r=>setTimeout(r,2000));
  var s = snap(), bl = window.__blitLog();
  var row = { t: t, Mi: Math.round((ic()-t0)/1e6), nat: delta(prev,s), blits: bl.length };
  if (tag) row.KEY = tag;
  if (bl.length !== nb){ row.newBlits = bl.slice(nb).map(function(e){ return JSON.stringify(e); }); nb = bl.length; }
  prev = s; buckets.push(row); return row;
}
var surf0 = surface();
await window.__shot('t0-before');
for (var i=0;i<30;i++) await tick(i*2);
var surfMid = surface();
await window.__shot('t60-no-key');
var pressed = window.__key(0x7D, 0);
for (var j=0;j<10;j++) await tick(60+j*2, j===0 ? 'guide 0x7D pressed' : undefined);
await window.__shot('t80-after-key');
return { pressed: pressed, surfaceAtStart: surf0, surfaceAt60s: surfMid, surfaceAtEnd: surface(),
         buckets: buckets.filter(function(b){ return b.nat.length || b.newBlits || b.KEY; }),
         quietBuckets: buckets.filter(function(b){ return !(b.nat.length || b.newBlits || b.KEY); }).length,
         tasks: window.__tasks().n };
