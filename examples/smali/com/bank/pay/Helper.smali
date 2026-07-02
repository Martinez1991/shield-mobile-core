.class public Lcom/bank/pay/Helper;
.super Ljava/lang/Object;

.method public static ping()V
    .registers 1
    const-string v0, "balance-check-ok"
    return-void
.end method

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
    if-lez v0, :neg
    return v0
    :neg
    const/4 v0, 0x0
    return v0
.end method

.method public static mix(II)I
    .registers 4
    mul-int v0, p0, p0
    add-int v0, v0, p1
    add-int/lit8 v0, v0, 0x7
    return v0
.end method
