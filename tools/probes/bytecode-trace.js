// Capture a raw o-code execution trace from the running EPG.
//
// Every read of the EPG module's CODE chunk, with the PC of the instruction that made it. The
// interpreter's main fetch site (0x80069298) reads OPCODE bytes; every other site reads OPERANDS.
// Walking the trace in icount order therefore yields, for each opcode byte, exactly how many
// operand bytes it consumed -- the operand table, measured off the running machine rather than
// guessed from 209 handlers. Feed the output to scripts/ocode-disasm.py.
await new Promise(r => setTimeout(r, 35000));   // let the box finish booting
window.__readWatch(0x9FC4A400, 0x9FC4A400 + 353188);
window.__key(0x21, 0);                          // one key press is enough: ~1000 reads
await new Promise(r => setTimeout(r, 2500));
var lg = window.__readWatchLog();
window.__readWatch();
return { reads: lg.reads, trace: lg.all.map(function (e) { return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }) };
