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
