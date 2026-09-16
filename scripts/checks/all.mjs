// Runs every page check in turn: node scripts/checks/all.mjs
//
// Each check starts its own Chrome and its own static server on its own port,
// so they can also be run one by one while working on the page.
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL("./", import.meta.url));
const checks = ["language.mjs", "offline.mjs", "validation.mjs", "saved-runs.mjs", "slider.mjs", "startapp.mjs"];

const failed = [];
for (const check of checks) {
    console.log(`\n=== ${check}`);
    const run = spawnSync(process.execPath, [join(here, check)], { stdio: "inherit" });
    if (run.status !== 0) {
        failed.push(check);
    }
}

console.log(failed.length ? `\nfailed: ${failed.join(", ")}` : "\nevery check passed");
process.exit(failed.length ? 1 : 0);
