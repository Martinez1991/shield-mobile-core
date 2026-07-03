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
