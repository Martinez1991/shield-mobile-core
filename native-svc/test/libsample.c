// A shared-library entry point with real control flow + integer arithmetic, so
// that a semantics-breaking flatten/mba would change its result. Exported with
// default visibility so a dlopen driver (host) can call it.
#include <stdint.h>

__attribute__((visibility("default"))) int compute(int n) {
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
  return acc ^ (int)((uint32_t)n * 2654435761u);
}
