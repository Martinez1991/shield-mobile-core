// Minimal ptrace tracer for the RASP gate: fork, PTRACE_TRACEME + exec the
// target, then continue it and propagate its exit code. Because the target is
// traced, its /proc/self/status TracerPid is nonzero, so the injected anti-debug
// check fires. Self-contained, so the gate needs no strace/gdb.
#include <stdio.h>
#include <sys/ptrace.h>
#include <sys/wait.h>
#include <unistd.h>

int main(int argc, char **argv) {
  if (argc < 2) {
    fprintf(stderr, "usage: %s <program> [args...]\n", argv[0]);
    return 2;
  }
  pid_t child = fork();
  if (child == 0) {
    ptrace(PTRACE_TRACEME, 0, 0, 0);
    execv(argv[1], &argv[1]);
    _exit(127); // exec failed
  }
  int status;
  waitpid(child, &status, 0); // initial stop at exec
  ptrace(PTRACE_CONT, child, 0, 0);
  while (waitpid(child, &status, 0) > 0) {
    if (WIFEXITED(status))
      return WEXITSTATUS(status);
    if (WIFSIGNALED(status))
      return 128 + WTERMSIG(status);
    ptrace(PTRACE_CONT, child, 0, 0); // deliver no signal, keep going
  }
  return 0;
}
