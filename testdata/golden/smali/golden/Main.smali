.class public Lgolden/Main;
.super Ljava/lang/Object;

# Entry point for the differential correctness gate (issue #3). Runs the golden
# methods with fixed inputs and prints deterministic output. The ORIGINAL and the
# PROTECTED dex must print byte-identical lines when run on ART (app_process).
# Kept un-renamed (golden.Main) so app_process can name it.

.method public static main([Ljava/lang/String;)V
    .registers 5
    sget-object v0, Ljava/lang/System;->out:Ljava/io/PrintStream;

    const/4 v1, 0x3
    const/4 v2, 0x4
    invoke-static {v1, v2}, Lgolden/Calc;->mix(II)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    const/4 v1, 0x5
    invoke-static {v1}, Lgolden/Calc;->sum(I)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    invoke-static {}, Lgolden/Secrets;->token()Ljava/lang/String;
    move-result-object v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(Ljava/lang/String;)V

    const/16 v1, 0x7
    invoke-static {v1}, Lgolden/Logic;->classify(I)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    const/16 v1, 0x14
    invoke-static {v1}, Lgolden/Logic;->classify(I)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    return-void
.end method
