.class public Lgolden/Logic;
.super Ljava/lang/Object;

# classify has branches -> exercised by reordering + opaque predicates.
# classify(7) = 1 ; classify(20) = 100
.method public static classify(I)I
    .registers 2
    const/16 v0, 0xa
    if-lt p0, v0, :small
    const/16 v0, 0x64
    return v0
    :small
    const/4 v0, 0x1
    return v0
.end method

# choose exercises object plumbing under virtualization: two reference (String)
# params, an int selector, a branch, move-object and return-object. Returns p0
# when the selector is > 0, else p1. choose("AA","BB",1)="AA"; (..,-1)="BB".
.method public static choose(Ljava/lang/String;Ljava/lang/String;I)Ljava/lang/String;
    .registers 4
    if-lez p2, :second
    move-object v0, p0
    return-object v0
    :second
    move-object v0, p1
    return-object v0
.end method

# tag exercises const-string virtualization: the string literals are lifted into
# the VM's string pool (invisible to a static const-string scan), and — because
# virtualization runs before string encryption — the pooled literals are then
# themselves AES-encrypted. tag(1)="pos"; tag(-1)="neg".
.method public static tag(I)Ljava/lang/String;
    .registers 2
    if-lez p0, :neg
    const-string v0, "pos"
    return-object v0
    :neg
    const-string v0, "neg"
    return-object v0
.end method

# absOf is a plain int helper with a branch -> virtualized by the VM.
.method public static absOf(I)I
    .registers 2
    if-gez p0, :p
    neg-int v0, p0
    return v0
    :p
    return p0
.end method

# score has real control flow plus a call to an OWNED helper (absOf). The VM
# refuses it (name-based reflection can't survive renaming an owned callee), so
# it is exercised by CONTROL-FLOW FLATTENING instead: the blocks become cases of
# a central packed-switch dispatcher. Pure int, so the typed-IR gate allows it.
# score(5)=|5-10|=5; score(-3)=0.
.method public static score(I)I
    .registers 3
    if-lez p0, :neg
    add-int/lit8 v0, p0, -0xa
    invoke-static {v0}, Lgolden/Logic;->absOf(I)I
    move-result v0
    return v0
    :neg
    const/4 v0, 0x0
    return v0
.end method

# maxOf calls the EXTERNAL Math.max, so the VM virtualizes it and the call is
# performed by reflection inside the interpreter (data-driven invoke).
# maxOf(42,7)=42.
.method public static maxOf(II)I
    .registers 3
    invoke-static {p0, p1}, Ljava/lang/Math;->max(II)I
    move-result v0
    return v0
.end method

# tagAbs holds a REFERENCE register (v1) across the central dispatcher: it calls
# an owned helper (so the VM refuses it) and branches to return one of two
# strings. Reference flattening (#48) — the typed-IR gate now allows it because
# every register keeps a single consistent type. tagAbs(5)="nonzero"; tagAbs(0)="zero".
.method public static tagAbs(I)Ljava/lang/String;
    .registers 4
    invoke-static {p0}, Lgolden/Logic;->absOf(I)I
    move-result v0
    if-lez v0, :z
    const-string v1, "nonzero"
    return-object v1
    :z
    const-string v1, "zero"
    return-object v1
.end method

# maxL: invoke-static with LONG args and a long return (#49). maxL(5e9,3)=5e9.
.method public static maxL(JJ)J
    .registers 6
    invoke-static {p0, p1, p2, p3}, Ljava/lang/Math;->max(JJ)J
    move-result-wide v0
    return-wide v0
.end method

# strOf: invoke-static with an OBJECT return (#49). strOf(42)="42".
.method public static strOf(I)Ljava/lang/String;
    .registers 2
    invoke-static {p0}, Ljava/lang/String;->valueOf(I)Ljava/lang/String;
    move-result-object v0
    return-object v0
.end method

# parseFixed: invoke-static with an OBJECT arg (a pooled const-string) (#49). The
# int param is ignored (the VM only virtualizes methods with >=1 param).
# parseFixed(0)=123.
.method public static parseFixed(I)I
    .registers 3
    const-string v0, "123"
    invoke-static {v0}, Ljava/lang/Integer;->parseInt(Ljava/lang/String;)I
    move-result v1
    return v1
.end method

# strLen: invoke-VIRTUAL on a receiver, no args, int return (#50). The call
# resolves dynamically via reflection on the String receiver. strLen("hello")=5.
.method public static strLen(Ljava/lang/String;)I
    .registers 2
    invoke-virtual {p0}, Ljava/lang/String;->length()I
    move-result v0
    return v0
.end method

# cat: invoke-VIRTUAL with a receiver + object arg + object return (#50).
# cat("foo","bar")="foobar".
.method public static cat(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;
    .registers 3
    invoke-virtual {p0, p1}, Ljava/lang/String;->concat(Ljava/lang/String;)Ljava/lang/String;
    move-result-object v0
    return-object v0
.end method
