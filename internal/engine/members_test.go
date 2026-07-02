package engine

import (
	"strings"
	"testing"

	"shield/internal/smali"
)

const svcSmali = `.class public Lcom/bank/Svc;
.super Ljava/lang/Object;

.field private secret:Ljava/lang/String;
.field public shared:I

.method public onCreate()V
    .registers 1
    return-void
.end method

.method private helper(I)I
    .registers 3
    iput p1, p0, Lcom/bank/Svc;->shared:I
    return p1
.end method

.method public static util(I)I
    .registers 2
    add-int/lit8 v0, p0, 0x1
    return v0
.end method
`

const callerSmali = `.class public Lcom/bank/Caller;
.super Ljava/lang/Object;

.method public run()V
    .registers 2
    const/4 v0, 0x5
    invoke-static {v0}, Lcom/bank/Svc;->util(I)I
    move-result v0
    return-void
.end method
`

func TestMemberRenaming(t *testing.T) {
	svc := &smali.Class{Descriptor: "Lcom/bank/Svc;", Lines: strings.Split(svcSmali, "\n")}
	caller := &smali.Class{Descriptor: "Lcom/bank/Caller;", Lines: strings.Split(callerSmali, "\n")}
	classes := []*smali.Class{svc, caller}

	n := passRenameMembers(classes, []string{"com/bank"}, nil)
	// eligible: private field `secret`, private method `helper`, static method `util`.
	if n != 3 {
		t.Fatalf("membersRenamed = %d, want 3", n)
	}

	svcBody := strings.Join(svc.Lines, "\n")
	callerBody := strings.Join(caller.Lines, "\n")

	// Public virtual method and public field must be UNTOUCHED.
	if !strings.Contains(svcBody, "onCreate()V") {
		t.Error("public virtual onCreate was renamed (would break framework dispatch)")
	}
	if !strings.Contains(svcBody, "shared:I") {
		t.Error("public field shared was renamed")
	}

	// Private members must be renamed at declaration.
	if strings.Contains(svcBody, "secret:Ljava/lang/String;") {
		t.Error("private field secret not renamed")
	}
	if strings.Contains(svcBody, "helper(I)I") {
		t.Error("private method helper not renamed")
	}

	// The cross-class static call must be rewritten consistently.
	if strings.Contains(callerBody, "->util(I)I") {
		t.Error("static call to util not rewritten in caller")
	}
	// Whatever util became, caller and declaration must agree.
	declName := memberNewName(t, svcBody, "static", "(I)I")
	if !strings.Contains(callerBody, "Lcom/bank/Svc;->"+declName+"(I)I") {
		t.Errorf("caller call site does not match renamed util (%q):\n%s", declName, callerBody)
	}
}

func TestMemberRenamingSkipsEnum(t *testing.T) {
	enum := &smali.Class{Descriptor: "Lcom/bank/E;", Lines: strings.Split(
		`.class public final enum Lcom/bank/E;
.super Ljava/lang/Enum;

.method private values2()V
    .registers 1
    return-void
.end method
`, "\n")}
	if n := passRenameMembers([]*smali.Class{enum}, []string{"com/bank"}, nil); n != 0 {
		t.Errorf("enum members should be skipped, got %d", n)
	}
}

func TestMemberRenamingRespectsKeep(t *testing.T) {
	svc := &smali.Class{Descriptor: "Lcom/bank/Svc;", Lines: strings.Split(svcSmali, "\n")}
	n := passRenameMembers([]*smali.Class{svc}, []string{"com/bank"}, []string{"com.bank.Svc"})
	if n != 0 {
		t.Errorf("kept class members should not be renamed, got %d", n)
	}
}

// memberNewName finds the new name of the static (I)I method in a class body.
func memberNewName(t *testing.T, body, flag, sig string) string {
	t.Helper()
	for _, ln := range strings.Split(body, "\n") {
		m := methodDeclRE.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		if strings.Contains(m[2], flag) && m[4] == sig {
			return m[3]
		}
	}
	t.Fatalf("could not find %s method %s in body", flag, sig)
	return ""
}
