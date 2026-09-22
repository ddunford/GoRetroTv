// Prepare the Digibox application image for analysis: map the flash beside it, and seed the
// MIPS16 entry points that flow analysis expands from.
//
// WHY BOTH BLOCKS. The running program is not one file. The application executes from a RAM image
// the bootloader decompresses to 0x800009F4; its strings, its jump tables and every pool word it
// loads with `lw $rx,n($pc)` live in the flash chip at 0x9FC00000. Import one without the other and
// half of every function is an unresolved pointer. FLASH_U202.bin is the chip that holds the reset
// vector -- 0xBFC00000, mirrored into KSEG0 at 0x9FC00000, which is the window the application's
// own code quotes and therefore the one to map.
//
// WHY SEEDS RATHER THAN A BLANKET ISA MODE. The image is mixed MIPS32 and MIPS16 reached through
// JALX, so setting ISA_MODE across the whole thing would decode every MIPS32 function into
// plausible nonsense -- and plausible nonsense is the one failure this project cannot afford, because
// it does not announce itself. Each seed passed in was reached by TRACING the running machine, so
// each is known to be MIPS16 and known to be a function entry.
//
// Arguments: <repo-root> <seed-address> [<seed-address> ...]
//
//@category Digibox
import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.math.BigInteger;

import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.Address;
import ghidra.program.model.lang.Register;
import ghidra.program.model.mem.Memory;

public class DigiboxSetup extends GhidraScript {

	private static final long FLASH_U202_BASE = 0x9FC00000L;
	private static final String FLASH_U202 = "firmware/FLASH_U202.bin";

	@Override
	public void run() throws Exception {
		String[] args = getScriptArgs();
		if (args.length < 1) {
			println("DigiboxSetup: usage <repo-root> [seed ...] -- nothing done");
			return;
		}
		mapFlash(new File(args[0], FLASH_U202));
		seedMips16(args);
	}

	// The flash is mapped read-only: it is a ROM, and a block Ghidra believes is writable invites
	// the decompiler to treat its constants as variables.
	private void mapFlash(File image) throws Exception {
		if (!image.isFile()) {
			println("DigiboxSetup: " + image + " not found; pool words and strings will be "
				+ "unresolved. It is gitignored like the rest of the firmware -- copy it in.");
			return;
		}
		Memory memory = currentProgram.getMemory();
		Address base = toAddr(FLASH_U202_BASE);
		if (memory.getBlock(base) != null) {
			println("DigiboxSetup: flash block already present at " + base);
			return;
		}
		try (InputStream in = new FileInputStream(image)) {
			memory.createInitializedBlock("flash_u202", base, in, image.length(), monitor, false)
				.setWrite(false);
		}
		println("DigiboxSetup: mapped " + image.getName() + " (" + image.length()
			+ " bytes) at " + base);
	}

	// ISA_MODE is the context register the MIPS:BE:32:16e language switches decoding on. Setting it
	// at an address and disassembling from there is what tells Ghidra "this one is MIPS16"; the
	// analyser then follows the flow out of it.
	private void seedMips16(String[] args) throws Exception {
		Register isaMode = currentProgram.getRegister("ISA_MODE");
		if (isaMode == null) {
			println("DigiboxSetup: this program has no ISA_MODE register -- is the processor "
				+ "MIPS:BE:32:16e? No seeds applied.");
			return;
		}
		int seeded = 0;
		for (int i = 1; i < args.length; i++) {
			Address at = toAddr(Long.decode(args[i]).longValue() & ~1L);
			currentProgram.getProgramContext().setValue(isaMode, at, at, BigInteger.ONE);
			disassemble(at);
			if (getFunctionAt(at) == null) {
				createFunction(at, null);
			}
			seeded++;
		}
		println("DigiboxSetup: seeded " + seeded + " MIPS16 entry points");
	}
}
