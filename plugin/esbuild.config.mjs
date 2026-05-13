import esbuild from "esbuild";

const production = process.argv.includes("production");

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
