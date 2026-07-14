// Command shield-nativeapk protects the recompilable native modules of an
// APK/AAB (issue #82/#64): it wires the native-svc Go seam (internal/nativesvc)
// to a real linker and the anti-tamper patcher, transforming each
// "lib/<abi>/<name>.so.bc" bitcode sidecar with native-svc, linking it back to a
// .so, applying the post-link tamper patch when the tamper pass is used, and
// repackaging. This is the worker-side end of ADR 0004; the LLVM toolchain lives
// in native-svc and the clang/python it shells out to, never in the engine.
//
//	shield-nativeapk --in app.apk --out out.apk --passes "flatten mba opaque strings" \
//	    --native-svc ./native-svc/build/native-svc --cc clang-18 \
//	    --python python3 --patch ./native-svc/tools/tamper-patch.py
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"shield/internal/nativesvc"
)

func main() {
	in := flag.String("in", "", "input .apk/.aab")
	out := flag.String("out", "", "output artifact")
	passesStr := flag.String("passes", "flatten mba opaque strings", "space-separated native-svc passes")
	seed := flag.Int64("seed", 1, "per-build seed")
	svc := flag.String("native-svc", "native-svc", "native-svc executable (or $SHIELD_NATIVE_SVC)")
	cc := flag.String("cc", "clang-18", "clang used to link bitcode into a .so")
	python := flag.String("python", "python3", "python used for the tamper patcher")
	patch := flag.String("patch", "native-svc/tools/tamper-patch.py", "tamper-patch.py path")
	flag.Parse()

	if *in == "" || *out == "" {
		die("--in and --out are required")
	}
	passes, err := nativesvc.ParsePasses(strings.Fields(*passesStr))
	if err != nil {
		die("%v", err)
	}
	s, err := nativesvc.New(nativesvc.Config{Passes: passes, Seed: *seed, Exec: *svc})
	if err != nil {
		die("native-svc: %v", err)
	}

	link := nativesvc.ClangLinker(*cc)
	patcher := nativesvc.ScriptPatcher(*python, *patch)

	res, err := s.ProtectArchive(*in, *out, link, patcher)
	if err != nil {
		die("protect: %v", err)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "shield-nativeapk: "+format+"\n", a...)
	os.Exit(1)
}
