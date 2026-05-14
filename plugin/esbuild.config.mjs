import esbuild from "esbuild";
import { copyFile, mkdir } from "node:fs/promises";

const production = process.argv.includes("production");

await mkdir("dist", { recursive: true });

await esbuild.build({
  banner: {
    js: "/* NoX Sync Obsidian plugin */",
  },
  bundle: true,
  entryPoints: ["src/main.ts"],
  external: ["obsidian", "electron", "@codemirror/*"],
  format: "cjs",
  logLevel: "info",
  minify: production,
  outfile: "dist/main.js",
  platform: "browser",
  sourcemap: production ? false : "inline",
  target: "es2022",
  treeShaking: true,
});

await Promise.all([
  copyFile("manifest.json", "dist/manifest.json"),
  copyFile("styles.css", "dist/styles.css"),
]);
