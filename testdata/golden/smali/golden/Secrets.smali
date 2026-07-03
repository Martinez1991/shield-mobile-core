.class public Lgolden/Secrets;
.super Ljava/lang/Object;

# token returns a constant string -> exercised by string encryption (the value
# must be reconstructed identically at runtime).
.method public static token()Ljava/lang/String;
    .registers 1
    const-string v0, "golden-secret-42"
    return-object v0
.end method
