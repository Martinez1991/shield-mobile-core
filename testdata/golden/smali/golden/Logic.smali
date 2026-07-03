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
