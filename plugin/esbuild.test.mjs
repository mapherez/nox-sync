import esbuild from "esbuild";

await esbuild.build({
  bundle: true,
  entryPoints: ["test/sync-core.test.ts"],
  format: "cjs",
  logLevel: "info",
  outfile: ".test-dist/sync-core.test.cjs",
  platform: "node",
  sourcemap: "inline",
  target: "es2022",
});
