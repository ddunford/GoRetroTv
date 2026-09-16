// WHAT DOES THE APPLICATION READ BETWEEN PUTTING ITS SCREEN UP AND TAKING IT DOWN?
//
// MEASURED 2026-09-14 (scripts/digibox-probes/setwindowroot-args.js). The application does NOT
// fail to bind its widget tree to the visible plane. It binds it and then unbinds it:
//
//   164838860  setWindowRoot(1, 0x80430A14)   ra 0x80081D3B (the (1,0xE3) o-code shim)
//   164841592  setWindowRoot(1, 0x00000000)   ra 0x80081D3B  -- 2732 instructions later
//
// with the firmware's own internals confirming both halves -- setGateRecursive(0x80430A14,1) and
// setWindowRecursive(0x80430A14,1) on the way in, then (...,0) and (...,0xFFFFFFFF) on the way
// out. The key press 155M instructions later then builds its 62 widgets under a root that was
// detached long before, which is why every one of them carries gate 0 and window 0xFFFFFFFF.
//
// So the question is no longer "what binds a context to a window" -- that is answered, it is
// (1,0xE3), and performing its writes by hand takes DAMAGE from 0 to 48 calls carrying window 1
// and puts five real fills on the plane (scripts/digibox-probes/bind-the-tree.js). The question is
// what the application LEARNED in those 2732 instructions that made it take its own screen down.
//
// This probe answers it the only way that is sound: stop the machine at the first call, record
// every module-1 native between there and the second, and read the box's own printf across the
// same window. A native census is the application's vocabulary; its printf is its commentary.
//
// THE BREAK IS THE INSTRUMENT AND IT HAS A CONTROL. A stall that never happens looks exactly like
// a window with nothing in it, so this refuses to report unless it saw BOTH calls with the
// arguments above. Anything else is a harness failure, not a measurement.

var NAT = [
  [0,0x80081B00,0x80098F24],
  [1,0x80081B10,0x80098F58],
  [2,0x80081B20,0x800990CC],
  [3,0x80081B30,0x80099104],
  [4,0x800711EC,0x00000000],
  [5,0x80071214,0x00000000],
  [6,0x8007123C,0x00000000],
  [7,0x80071274,0x00000000],
  [8,0x800712BC,0x80071164],
  [9,0x800712CC,0x00000000],
  [10,0x800712FC,0x00000000],
  [11,0x8007132C,0x00000000],
  [12,0x80071360,0x00000000],
  [13,0x80071460,0x00000000],
  [14,0x80071420,0x80073428],
  [15,0x80071440,0x800735C0],
  [16,0x800714B0,0x800716F0],
  [17,0x800714C0,0x80071730],
  [18,0x80071520,0x80071770],
  [19,0x800714D0,0x8007193C],
  [20,0x800714E4,0x80071848],
  [21,0x800713C0,0x00000000],
  [22,0x80071530,0x00000000],
  [23,0x80071488,0x00000000],
  [24,0x800820E0,0x80082ECC],
  [25,0x800820C0,0x80082F44],
  [26,0x800820D0,0x80082F20],
  [27,0x800820F0,0x80082E54],
  [28,0x80082100,0x80082F64],
  [29,0x80082110,0x80085004],
  [30,0x80081C28,0x80083DC8],
  [31,0x80081C38,0x80083DEC],
  [32,0x80081C48,0x80083810],
  [33,0x80081C68,0x80083780],
  [34,0x80081CA8,0x80084008],
  [35,0x80081CB8,0x80084028],
  [36,0x80081CC8,0x80083F9C],
  [37,0x80081CD8,0x80083FD0],
  [38,0x80081CE8,0x80084050],
  [39,0x80081D10,0x8008409C],
  [40,0x80081D20,0x80085104],
  [41,0x80081D40,0x8008521C],
  [42,0x80081C88,0x80083B68],
  [43,0x80050F40,0x800513BC],
  [44,0x80050F50,0x8001E504],
  [45,0x80082040,0x800855AC],
  [46,0x80082050,0x80085668],
  [47,0x80081DD0,0x80085768],
  [48,0x80081DE0,0x800856E0],
  [49,0x80081DF0,0x800857DC],
  [50,0x80081DB0,0x80085530],
  [51,0x80081DC0,0x80085500],
  [52,0x80081D80,0x80085564],
  [53,0x80081D60,0x8008508C],
  [54,0x80081D70,0x800850B8],
  [55,0x80081F10,0x8008546C],
  [56,0x80081F20,0x80085004],
  [57,0x80081F44,0x800852A4],
  [58,0x80081F34,0x800852A4],
  [59,0x80082020,0x800852A4],
  [60,0x80081D90,0x80085578],
  [61,0x80081DA0,0x80085598],
  [62,0x80082124,0x80085004],
  [63,0x8008213C,0x80085004],
  [64,0x80082150,0x80084FE4],
  [65,0x80082160,0x80084FEC],
  [66,0x80082170,0x80084F84],
  [67,0x80081E00,0x8008589C],
  [68,0x80081E20,0x80085938],
  [69,0x80081E30,0x8008594C],
  [70,0x80081E40,0x80085964],
  [71,0x80081E50,0x80085A80],
  [72,0x80081E60,0x80085AD4],
  [73,0x80081E70,0x80085B0C],
  [74,0x80081E80,0x80085AE8],
  [75,0x80081E90,0x80085B2C],
  [76,0x80081EA0,0x80085B4C],
  [77,0x80081EB0,0x80085BDC],
  [78,0x80081ED0,0x80085CC8],
  [79,0x80081EE0,0x80085D10],
  [80,0x80081F00,0x80085D84],
  [81,0x80081EF0,0x80085DF4],
  [82,0x80082190,0x800859E0],
  [83,0x800821A0,0x80085A0C],
  [84,0x800821B0,0x80085A4C],
  [85,0x80081E10,0x8008506C],
  [86,0x800821C0,0x80085040],
  [87,0x80081570,0x80082A6C],
  [88,0x80081590,0x80082AFC],
  [89,0x80081580,0x80082AD8],
  [90,0x80081540,0x80082668],
  [91,0x80081550,0x80082690],
  [92,0x8008153C,0x80082668],
  [93,0x800816E4,0x800826B8],
  [94,0x80081714,0x800826E8],
  [95,0x80081560,0x80082790],
  [96,0x800816F4,0x800826E0],
  [97,0x80081704,0x800826E4],
  [98,0x80081640,0x80082738],
  [99,0x80081650,0x80082A2C],
  [100,0x800815B0,0x800829A4],
  [101,0x800815C0,0x80082D1C],
  [102,0x80081610,0x80082964],
  [103,0x80081620,0x80082D1C],
  [104,0x80081684,0x80082898],
  [105,0x80081694,0x80082D1C],
  [106,0x800816B4,0x800828DC],
  [107,0x800816C4,0x80082D1C],
  [108,0x800815E0,0x800829E8],
  [109,0x800815F0,0x80082D1C],
  [110,0x80081654,0x80082A2C],
  [111,0x80081664,0x80082D1C],
  [112,0x80081724,0x800827D0],
  [113,0x80081734,0x800827B0],
  [114,0x80081744,0x80082814],
  [115,0x80081754,0x80082834],
  [116,0x80081764,0x80082878],
  [117,0x800815A0,0x80080C54],
  [118,0x80081794,0x80080FD0],
  [119,0x800817A4,0x80080F28],
  [120,0x800817C4,0x80080F98],
  [121,0x80081774,0x8008101C],
  [122,0x800817B4,0x80080F60],
  [123,0x800817D4,0x80080F7C],
  [124,0x80081784,0x80081154],
  [125,0x800817E4,0x80083230],
  [126,0x80081804,0x800832F0],
  [127,0x800817F4,0x800832BC],
  [128,0x80081814,0x8008327C],
  [129,0x80081824,0x8008319C],
  [130,0x80081844,0x80082D1C],
  [131,0x80081834,0x800831F0],
  [132,0x80081864,0x8008335C],
  [133,0x80081874,0x80082D1C],
  [134,0x80081950,0x800833EC],
  [135,0x80081960,0x80082D1C],
  [136,0x80081980,0x80083444],
  [137,0x80081990,0x80082D1C],
  [138,0x800819B0,0x80083490],
  [139,0x800819C0,0x80082D1C],
  [140,0x80081930,0x800833B4],
  [141,0x80081940,0x8008329C],
  [142,0x80081A10,0x8008229C],
  [143,0x80081A30,0x80082298],
  [144,0x80081A40,0x800822E8],
  [145,0x80081A50,0x80082D1C],
  [146,0x80081A20,0x800822C8],
  [147,0x80081A70,0x80082468],
  [148,0x80081A90,0x80082394],
  [149,0x80081AB0,0x800823E0],
  [150,0x80081A80,0x8008242C],
  [151,0x80081AA0,0x800823C0],
  [152,0x80081AC0,0x8008240C],
  [153,0x80081AD0,0x80082530],
  [154,0x80081AE0,0x80082578],
  [155,0x80081AF0,0x80082558],
  [156,0x80082060,0x80084530],
  [157,0x80082080,0x800845E4],
  [158,0x80082070,0x800846E4],
  [159,0x800865FC,0x80086CAC],
  [160,0x800865EC,0x80086C60],
  [161,0x8008660C,0x80086CF8],
  [162,0x8008663C,0x80086F48],
  [163,0x8008661C,0x80086E2C],
  [164,0x8008662C,0x80086EE8],
  [165,0x8008664C,0x80087064],
  [166,0x8009CDC4,0x8009CE00],
  [167,0x8009CDD4,0x8009CE2C],
  [168,0x8009CDE4,0x8009CE54],
  [169,0x80085F04,0x800861C4],
  [170,0x80085F14,0x8008621C],
  [171,0x80081244,0x80081200],
  [172,0x8008125C,0x80081200],
  [173,0x80081274,0x00000000],
  [174,0x80081218,0x00000000],
  [175,0x80081298,0x00000000],
  [176,0x800812BC,0x00000000],
  [177,0x800812E0,0x80081200],
  [178,0x80081350,0x80081200],
  [179,0x80081450,0x80081200],
  [180,0x80081438,0x80081200],
  [181,0x800812F8,0x00000000],
  [182,0x80081388,0x00000000],
  [183,0x800813E0,0x00000000],
  [184,0x80081488,0x80081200],
  [185,0x800814C8,0x80081200],
  [186,0x80082180,0x80084080],
  [187,0x8009CCB4,0x80080668],
  [188,0x8009CCCC,0x00000000],
  [189,0x8009CD00,0x8006734C],
  [190,0x8009CD2C,0x80067454],
  [191,0x8009CD40,0x800672F4],
  [192,0x80086414,0x80086480],
  [193,0x80086424,0x80086490],
  [194,0x80086434,0x800864E4],
  [195,0x80071500,0x800717AC],
  [196,0x80071510,0x80071778],
  [197,0x800819E0,0x80082D1C],
  [198,0x80081A00,0x800834DC],
  [199,0x80081B40,0x80099244],
  [200,0x80081B50,0x80099288],
  [201,0x80081B60,0x80099468],
  [202,0x80081B70,0x800994AC],
  [203,0x80081B80,0x80083528],
  [204,0x80081B90,0x80082D1C],
  [205,0x80071630,0x80073BD4],
  [206,0x80071650,0x80074064],
  [207,0x80071558,0x00000000],
  [208,0x80071590,0x8007228C],
  [209,0x80071610,0x80072DF4],
  [210,0x80082090,0x80084388],
  [211,0x800820B0,0x80084554],
  [212,0x800820A0,0x80084604],
  [213,0x8007138C,0x00000000],
  [214,0x800713EC,0x00000000],
  [215,0x80071430,0x8007350C],
  [216,0x80071450,0x800735EC],
  [217,0x80071578,0x00000000],
  [218,0x800715E0,0x80072C90],
  [219,0x80071534,0x00000000],
  [220,0x80071548,0x80072BF0],
  [221,0x800715C0,0x80072700],
  [222,0x80081D50,0x80085248],
  [223,0x800715A0,0x800724C0],
  [224,0x80081C78,0x80083644],
  [225,0x800715D0,0x80072C2C],
  [226,0x800715B0,0x800725D8],
  [227,0x80081D30,0x80085128],
  [228,0x80081C58,0x800837A0],
  [229,0x80081C98,0x80083C44],
  [230,0x800715F0,0x80072CEC],
  [231,0x80071600,0x80072DA0],
  [232,0x80081EC0,0x80085CEC],
  [233,0x80071620,0x800738D8],
  [234,0x80071640,0x80073C0C],
  [235,0x80081CF8,0x00000000]
];

function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
var SHIM = {};
NAT.forEach(function(r){ SHIM[r[1]>>>0] = r[0]; });
function natName(pc){
  var id = SHIM[parseInt(pc,16)>>>0];
  return id === undefined ? pc : '(1,0x'+id.toString(16).toUpperCase().padStart(2,'0')+')';
}

window.__profile(true);
var SET_WINDOW_ROOT = 0x80085128;

async function waitForStall(limitMs){
  var t0 = Date.now();
  while (Date.now() - t0 < limitMs){
    await new Promise(function(r){ setTimeout(r, 4); });
    var g = window.__regs();
    if ((parseInt(g.pc, 16)>>>0) === SET_WINDOW_ROOT) return g;
  }
  return null;
}

window.__breakAt([SET_WINDOW_ROOT]);
var first = await waitForStall(90000);
if (!first) throw new Error('never stalled at setWindowRoot -- nothing measured');
var firstArgs = [parseInt(first.a0,16)>>>0, parseInt(first.a1,16)>>>0];

// Arm the full module-1 census only for the gap, so the 4000-entry log holds that gap and
// nothing else.
window.__traceCalls(NAT.map(function(r){ return { pc: r[1], name: h(r[1]), args: 4 }; })
                       .concat([{ pc: SET_WINDOW_ROOT, name: 'setWindowRoot', args: 2 }]));
window.__traceClear();
var fwBefore = window.__fwLog().length;
window.__resume();

var second = await waitForStall(90000);
if (!second) throw new Error('never stalled at the SECOND setWindowRoot -- the gap was not captured');
var secondArgs = [parseInt(second.a0,16)>>>0, parseInt(second.a1,16)>>>0];

var log = window.__traceLog();
window.__breakAt([]);
window.__traceCalls([]);
window.__resume();

var seq = log.map(function(e){
  return { icount: e.icount, nat: e.name === 'setWindowRoot' ? 'setWindowRoot' : natName(e.name),
           args: e.a.join(',') };
});
var counts = {};
seq.forEach(function(e){ counts[e.nat] = (counts[e.nat]||0)+1; });

return {
  control: { firstCall: firstArgs.map(h).join(','), secondCall: secondArgs.map(h).join(','),
             gapInstructions: parseInt(second.icount || 0, 10) || null,
             bindThenUnbind: firstArgs[0] === 1 && firstArgs[1] !== 0 &&
                             secondArgs[0] === 1 && secondArgs[1] === 0 },
  nativesInGap: { total: seq.length, distinct: Object.keys(counts).length, counts: counts },
  sequence: seq.slice(0, 120),
  printfInGap: window.__fwLog().slice(fwBefore).map(function(e){ return e.n+'x '+e.text; }),
  printfTail: window.__fwLog().slice(-25).map(function(e){ return e.icount+' '+e.n+'x '+e.text; }),
  tasks: window.__tasks().n
};
