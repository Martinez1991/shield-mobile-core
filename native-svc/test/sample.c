// Sample with real control flow (branches + a loop) so that a semantics-breaking
// flattening would change the printed output — this is the execution-gate probe.
#include <stdio.h>

static int classify(int n) {
  int acc = 0;
  for (int i = 1; i <= n; i++) {
    if (i % 15 == 0)
      acc += 100;
    else if (i % 3 == 0)
      acc += 3;
    else if (i % 5 == 0)
      acc += 5;
    else
      acc += 1;
  }
  return acc;
}

int main(void) {
  for (int n = 0; n <= 40; n += 7)
    printf("classify(%d)=%d\n", n, classify(n));
  return 0;
}
