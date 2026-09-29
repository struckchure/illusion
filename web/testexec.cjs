// Runs a Go wasm binary under Node like Go's own wasm_exec_node.js, after
// loading every emscripten module in $ILLUSION_WASM_MODULES into
// globalThis.<name>, as the page does in a browser.
"use strict";

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

(async () => {
	const dir = process.env.ILLUSION_WASM_MODULES;
	for (const file of dir ? fs.readdirSync(dir).filter((f) => f.endsWith(".js")) : []) {
		const name = path.basename(file, ".js");
		globalThis[name] = await require(path.join(dir, file))();
	}
	const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
	require(path.join(goroot, "lib", "wasm", "wasm_exec_node.js"));
})().catch((err) => {
	console.error(err);
	process.exit(1);
});
