.class public Lcom/bank/pay/CardValidator;
.super Ljava/lang/Object;
.source "CardValidator.java"

.method public constructor <init>()V
    .registers 1
    .prologue
    .line 5
    invoke-direct {p0}, Ljava/lang/Object;-><init>()V
    return-void
.end method

.method public validate(Ljava/lang/String;)Z
    .registers 4
    .param p1, "card"    # Ljava/lang/String;
    .prologue
    .line 12
    const-string v0, "sk_live_9f2a3b4c5d6e7f8a"
    .line 13
    invoke-static {}, Lcom/bank/pay/Helper;->ping()V
    .line 15
    const/4 v1, 0x1
    return v1
.end method
