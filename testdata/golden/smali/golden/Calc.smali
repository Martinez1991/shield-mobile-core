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

# bits exercises the extended integer ALU (shifts, div/rem, neg/not, lit forms,
# rsub) under code virtualization. bits(6,2) = 7.
.method public static bits(II)I
    .registers 6
    shl-int v0, p0, p1
    shr-int v1, p0, p1
    ushr-int v2, p0, p1
    add-int v0, v0, v1
    add-int v0, v0, v2
    div-int/lit8 v0, v0, 0x2
    rem-int/lit8 v3, v0, 0x5
    xor-int v0, v0, v3
    neg-int v0, v0
    not-int v0, v0
    rsub-int/lit8 v0, v0, 0x14
    and-int/lit8 v0, v0, 0xf
    return v0
.end method

# narrow exercises const/high16 and the narrowing conversions (byte/short/char).
# narrow(0xABCD) = -51 + -21555 + 43981 = 22375
.method public static narrow(I)I
    .registers 4
    const/high16 v0, 0x12340000
    or-int v0, v0, p0
    int-to-short v1, v0
    int-to-char v2, v0
    int-to-byte v0, v0
    add-int v0, v0, v1
    add-int v0, v0, v2
    return v0
.end method

# wide exercises 64-bit long ops (mul/and/or/xor/not/add/move/neg/sub-long, i2l,
# return-wide). wide(100000,100000) overflows 32 bits, proving true 64-bit width.
# = 2 * (A*B + (A&B) + (A|B) + (A^B) + ~B) = 20000000199998
.method public static wide(II)J
    .registers 12
    int-to-long v0, p0
    int-to-long v2, p1
    mul-long v4, v0, v2
    and-long v6, v0, v2
    add-long v4, v4, v6
    or-long v6, v0, v2
    add-long v4, v4, v6
    xor-long v6, v0, v2
    add-long v4, v4, v6
    not-long v6, v2
    add-long v4, v4, v6
    move-wide v8, v4
    neg-long v8, v8
    sub-long v4, v4, v8
    return-wide v4
.end method

# wide2 exercises const-wide + div-long/rem-long + shl-long. wide2(3,4) = 736.
.method public static wide2(II)J
    .registers 10
    int-to-long v0, p0
    int-to-long v2, p1
    const-wide/16 v4, 0x64
    mul-long v0, v0, v4
    add-long v0, v0, v2
    const-wide/16 v4, 0x7
    div-long v6, v0, v4
    rem-long v8, v0, v4
    add-long v6, v6, v8
    shl-long v6, v6, p1
    return-wide v6
.end method
