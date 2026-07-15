// Command shield-iosstrip strips the Mach-O metadata (symbols / superfluous
// __LINKEDIT) of an IPA's app + framework binaries (issue #76) and repackages it.
// It shells out to the platform strip and (optionally) codesign — those tools
// only exist on macOS — while the IPA round-trip stays pure Go. Stripping breaks
// the code signature, so on Apple Silicon pass an ad-hoc --sign to keep the
// binary runnable; full distribution re-signing is #77.
//
//	shield-iosstrip --in app.ipa --out out.ipa \
//	    --strip "xcrun strip -x" --sign "codesign -f -s -"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"shield/internal/ios"
)

func main() {
	in := flag.String("in", "", "input .ipa")
	out := flag.String("out", "", "output .ipa")
	stripCmd := flag.String("strip", "xcrun strip -x", "strip command (Mach-O path appended)")
	signCmd := flag.String("sign", "", "ad-hoc re-sign command (empty = leave unsigned)")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "shield-iosstrip: --in and --out are required")
		os.Exit(1)
	}
	var sign ios.Signer
	if *signCmd != "" {
		sign = ios.ShellSigner(*signCmd)
	}
	res, err := ios.StripIPA(*in, *out, ios.ShellStripper(*stripCmd), sign)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shield-iosstrip: %v\n", err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
}
