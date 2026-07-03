.class public Lgolden/Calc;
.super Ljava/lang/Object;

# mix: straight-line integer arithmetic -> exercised by code virtualization.
# mix(3,4) = 3*3 + 4 + 7 = 20
.method public static mix(II)I
    .registers 4
    mul-int v0, p0, p0
    add-int v0, v0, p1
    add-int/lit8 v0, v0, 0x7
    return v0
.end method

# sum: a loop with branches -> exercised by block reordering.
# sum(5) = 0+1+2+3+4 = 10
.method public static sum(I)I
    .registers 4
    const/4 v0, 0x0
    const/4 v1, 0x0
    :loop
    if-ge v1, p0, :done
    add-int/2addr v0, v1
    add-int/lit8 v1, v1, 0x1
    goto :loop
    :done
    return v0
.end method
