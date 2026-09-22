// Decompile the named addresses of the Digibox application image and print the C.
//
// WHY IT PRINTS RATHER THAN OPENS A WINDOW. Everything in this project is measured from a script or
// a test, and a reading nobody can reproduce is a reading nobody can check. This runs headless, so
// a decompilation can be quoted in a bead or a commit message with the command that produced it.
//
// WHAT IT IS NOT. Ghidra reads the image; the EMULATOR is the ground truth. Every structural claim
// this project has settled came from tracing the running machine, and the static reading has been
// backwards more than once -- the resolver's exit looked like 0xFFFFFFFB on the page and the box
// returned 2, because the arm that ran was not the arm that looked like the exit. Use this to read
// faster, then prove it by running it.
//
// Each block is fenced with `====` because the shell wrapper prints from the first one, which is
// how the decompiler's output is separated from Ghidra's startup chatter.
//
// Arguments: <address> [<address> ...]
//
//@category Digibox
import ghidra.app.decompiler.DecompInterface;
import ghidra.app.decompiler.DecompileOptions;
import ghidra.app.decompiler.DecompileResults;
import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.Address;
import ghidra.program.model.listing.Function;

public class DigiboxDump extends GhidraScript {

	private static final int TIMEOUT_SECONDS = 120;

	@Override
	public void run() throws Exception {
		String[] args = getScriptArgs();
		if (args.length == 0) {
			println("==== DigiboxDump: no addresses given");
			return;
		}
		DecompInterface decompiler = new DecompInterface();
		decompiler.setOptions(new DecompileOptions());
		if (!decompiler.openProgram(currentProgram)) {
			println("==== DigiboxDump: could not open the program: " + decompiler.getLastMessage());
			return;
		}
		try {
			for (String arg : args) {
				dump(decompiler, arg);
			}
		} finally {
			decompiler.dispose();
		}
	}

	private void dump(DecompInterface decompiler, String arg) {
		Address at;
		try {
			at = toAddr(Long.decode(arg).longValue() & ~1L);
		} catch (NumberFormatException bad) {
			println("==== " + arg + ": not an address");
			return;
		}
		// The address may be anywhere inside a function; decompiling wants the whole thing.
		Function function = getFunctionContaining(at);
		if (function == null) {
			// A seed the analyser never reached is worth saying so about rather than silently
			// printing nothing: it usually means the address is MIPS16 and was not seeded.
			println("==== " + at + ": no function here. If this is MIPS16, add it to the SEEDS in "
				+ "tools/ghidra/ghidra-import.sh and re-import.");
			return;
		}
		DecompileResults results = decompiler.decompileFunction(function, TIMEOUT_SECONDS, monitor);
		if (results == null || !results.decompileCompleted()) {
			String why = results == null ? "no result" : results.getErrorMessage();
			println("==== " + function.getName() + " @ " + function.getEntryPoint()
				+ ": decompilation failed: " + why);
			return;
		}
		println("==== " + function.getName() + " @ " + function.getEntryPoint()
			+ "  (asked for " + at + ")");
		println(results.getDecompiledFunction().getC());
	}
}
