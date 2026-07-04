// Host driver for the native-lib execution gate: dlopen a shared object, resolve
// compute(), and print its result for a range of inputs. Comparing the output of
// the unprotected vs protected .so proves the transform is functionally identical.
#include <dlfcn.h>
#include <stdio.h>

int main(int argc, char **argv) {
  if (argc < 2) {
    fprintf(stderr, "usage: %s <lib.so>\n", argv[0]);
    return 2;
  }
  void *h = dlopen(argv[1], RTLD_NOW);
  if (!h) {
    fprintf(stderr, "dlopen: %s\n", dlerror());
    return 1;
  }
  int (*compute)(int) = (int (*)(int))dlsym(h, "compute");
  if (!compute) {
    fprintf(stderr, "dlsym: %s\n", dlerror());
    return 1;
  }
  for (int n = 0; n <= 40; n += 7)
    printf("compute(%d)=%d\n", n, compute(n));
  dlclose(h);
  return 0;
}
