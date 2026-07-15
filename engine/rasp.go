package engine

import (
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// raspDescriptor is the injected RASP runtime (kept out of renaming).
const raspDescriptor = "Lshield/rt/RASP;"

// RASPClass builds the injected Lshield/rt/RASP; runtime (shield-platform.md
// section 6). It is a small, self-contained set of static detectors following
// the section 6.1 model: detection produces flags; the *reaction* is left to the
// host (deferred/decoupled), so there is no single `if (isRooted) exit()` to
// patch. Detectors:
//
//	isDebuggerConnected()Z  — Debug.isDebuggerConnected
//	isEmulator()Z           — Build fingerprint/model/hardware heuristics
//	isRooted()Z             — su/magisk artifact presence
//	flags()I                — bitmask: debugger=1, emulator=2, rooted=4
//
// This smali was validated to assemble to a valid DEX. Native tripwires
// (Frida/Xposed/anti-hook, integrity of .so, distributed reaction) live in the
// native SDK on the roadmap (section 6).
func RASPClass(base string) *smali.Class {
	src := `.class public Lshield/rt/RASP;
.super Ljava/lang/Object;

# SHIELD RASP runtime (generated). Detection -> flags; reaction deferred to host.

.method public static isDebuggerConnected()Z
    .locals 1
    invoke-static {}, Landroid/os/Debug;->isDebuggerConnected()Z
    move-result v0
    return v0
.end method

.method public static isEmulator()Z
    .locals 3
    sget-object v0, Landroid/os/Build;->FINGERPRINT:Ljava/lang/String;
    const-string v1, "generic"
    invoke-virtual {v0, v1}, Ljava/lang/String;->contains(Ljava/lang/CharSequence;)Z
    move-result v2
    if-nez v2, :yes
    sget-object v0, Landroid/os/Build;->MODEL:Ljava/lang/String;
    const-string v1, "sdk"
    invoke-virtual {v0, v1}, Ljava/lang/String;->contains(Ljava/lang/CharSequence;)Z
    move-result v2
    if-nez v2, :yes
    sget-object v0, Landroid/os/Build;->HARDWARE:Ljava/lang/String;
    const-string v1, "goldfish"
    invoke-virtual {v0, v1}, Ljava/lang/String;->contains(Ljava/lang/CharSequence;)Z
    move-result v2
    if-nez v2, :yes
    const/4 v0, 0x0
    return v0
    :yes
    const/4 v0, 0x1
    return v0
.end method

.method public static isRooted()Z
    .locals 3
    new-instance v0, Ljava/io/File;
    const-string v1, "/system/xbin/su"
    invoke-direct {v0, v1}, Ljava/io/File;-><init>(Ljava/lang/String;)V
    invoke-virtual {v0}, Ljava/io/File;->exists()Z
    move-result v2
    if-nez v2, :yes
    new-instance v0, Ljava/io/File;
    const-string v1, "/data/adb/magisk"
    invoke-direct {v0, v1}, Ljava/io/File;-><init>(Ljava/lang/String;)V
    invoke-virtual {v0}, Ljava/io/File;->exists()Z
    move-result v2
    if-nez v2, :yes
    const/4 v0, 0x0
    return v0
    :yes
    const/4 v0, 0x1
    return v0
.end method

.method public static hasXposed()Z
    .locals 1
    :try_start_0
    const-string v0, "de.robv.android.xposed.XposedBridge"
    invoke-static {v0}, Ljava/lang/Class;->forName(Ljava/lang/String;)Ljava/lang/Class;
    const/4 v0, 0x1
    return v0
    :try_end_0
    .catch Ljava/lang/Throwable; {:try_start_0 .. :try_end_0} :catch_0
    :catch_0
    const/4 v0, 0x0
    return v0
.end method

# Frida default control port. Not called from flags() by default (a blocking
# connect must run off the main thread); exposed for the host to schedule.
.method public static hasFridaPort()Z
    .locals 3
    :try_start_0
    new-instance v0, Ljava/net/Socket;
    const-string v1, "127.0.0.1"
    const/16 v2, 0x69a2
    invoke-direct {v0, v1, v2}, Ljava/net/Socket;-><init>(Ljava/lang/String;I)V
    invoke-virtual {v0}, Ljava/net/Socket;->close()V
    const/4 v0, 0x1
    return v0
    :try_end_0
    .catch Ljava/lang/Throwable; {:try_start_0 .. :try_end_0} :catch_0
    :catch_0
    const/4 v0, 0x0
    return v0
.end method

# Deferred reaction primitive (section 6.1). The host decides policy; this is the
# blunt fallback: terminate when any flag is set. Kept decoupled from detection.
.method public static react(I)V
    .locals 1
    if-eqz p0, :ok
    const/4 v0, 0x1
    invoke-static {v0}, Ljava/lang/System;->exit(I)V
    :ok
    return-void
.end method

.method public static flags()I
    .locals 2
    const/4 v0, 0x0
    invoke-static {}, Lshield/rt/RASP;->isDebuggerConnected()Z
    move-result v1
    if-eqz v1, :a
    or-int/lit8 v0, v0, 0x1
    :a
    invoke-static {}, Lshield/rt/RASP;->isEmulator()Z
    move-result v1
    if-eqz v1, :b
    or-int/lit8 v0, v0, 0x2
    :b
    invoke-static {}, Lshield/rt/RASP;->isRooted()Z
    move-result v1
    if-eqz v1, :c
    or-int/lit8 v0, v0, 0x4
    :c
    invoke-static {}, Lshield/rt/RASP;->hasXposed()Z
    move-result v1
    if-eqz v1, :d
    or-int/lit8 v0, v0, 0x8
    :d
    return v0
.end method
`
	return &smali.Class{
		Base:       base,
		Descriptor: raspDescriptor,
		Lines:      strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n"),
	}
}
