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

    const/4 v1, 0x6
    const/4 v2, 0x2
    invoke-static {v1, v2}, Lgolden/Calc;->bits(II)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    const v1, 0xabcd
    invoke-static {v1}, Lgolden/Calc;->narrow(I)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    const v1, 0x186a0
    const v2, 0x186a0
    invoke-static {v1, v2}, Lgolden/Calc;->wide(II)J
    move-result-wide v1
    invoke-virtual {v0, v1, v2}, Ljava/io/PrintStream;->println(J)V

    const/4 v1, 0x3
    const/4 v2, 0x4
    invoke-static {v1, v2}, Lgolden/Calc;->wide2(II)J
    move-result-wide v1
    invoke-virtual {v0, v1, v2}, Ljava/io/PrintStream;->println(J)V

    const-wide v1, 0x12a05f200L
    const/4 v3, 0x5
    invoke-static {v1, v2, v3}, Lgolden/Calc;->combine(JI)J
    move-result-wide v1
    invoke-virtual {v0, v1, v2}, Ljava/io/PrintStream;->println(J)V

    const-string v1, "AA"
    const-string v2, "BB"
    const/4 v3, 0x1
    invoke-static {v1, v2, v3}, Lgolden/Logic;->choose(Ljava/lang/String;Ljava/lang/String;I)Ljava/lang/String;
    move-result-object v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(Ljava/lang/String;)V

    const-string v1, "AA"
    const-string v2, "BB"
    const/4 v3, -0x1
    invoke-static {v1, v2, v3}, Lgolden/Logic;->choose(Ljava/lang/String;Ljava/lang/String;I)Ljava/lang/String;
    move-result-object v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(Ljava/lang/String;)V

    const/4 v1, 0x1
    invoke-static {v1}, Lgolden/Logic;->tag(I)Ljava/lang/String;
    move-result-object v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(Ljava/lang/String;)V

    const/4 v1, -0x1
    invoke-static {v1}, Lgolden/Logic;->tag(I)Ljava/lang/String;
    move-result-object v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(Ljava/lang/String;)V

    const/4 v1, 0x5
    invoke-static {v1}, Lgolden/Logic;->score(I)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    const/4 v1, -0x3
    invoke-static {v1}, Lgolden/Logic;->score(I)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    const/16 v1, 0x2a
    const/4 v2, 0x7
    invoke-static {v1, v2}, Lgolden/Logic;->maxOf(II)I
    move-result v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(I)V

    const/4 v1, 0x5
    invoke-static {v1}, Lgolden/Logic;->tagAbs(I)Ljava/lang/String;
    move-result-object v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(Ljava/lang/String;)V

    const/4 v1, 0x0
    invoke-static {v1}, Lgolden/Logic;->tagAbs(I)Ljava/lang/String;
    move-result-object v1
    invoke-virtual {v0, v1}, Ljava/io/PrintStream;->println(Ljava/lang/String;)V

    return-void
.end method
