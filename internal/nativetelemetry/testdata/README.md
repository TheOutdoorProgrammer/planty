# Native symbol fixture

The thin arm64 dSYM fixture contains debug information for `symbols.c`, generated with Apple clang and dsymutil. Its UUID is `a185c3af-a04b-37e6-89cc-f1c53f078adf`; `native_crash_site` resolves at text-relative offset `0x328` to `symbols.c:1`. Source paths are remapped to `/fixture`.

To regenerate from this directory, preserve the object until dsymutil completes:

```sh
xcrun clang -g -O0 -arch arm64 -fdebug-prefix-map="$PWD"=/fixture -ffile-prefix-map="$PWD"=/fixture -fdebug-compilation-dir=/fixture -c symbols.c -o /tmp/planty-native-symbols-fixture.o
xcrun clang -arch arm64 /tmp/planty-native-symbols-fixture.o -o /tmp/planty-native-symbols-fixture
xcrun dsymutil /tmp/planty-native-symbols-fixture -o /tmp/planty-native-symbols-fixture.dSYM
cp /tmp/planty-native-symbols-fixture.dSYM/Contents/Resources/DWARF/planty-native-symbols-fixture symbols.dwarf
```

Compiler changes can change the UUID and offset. Update the fixture assertions after checking `dwarfdump --uuid` and `llvm-symbolizer`. The normal Go suite validates the Mach-O fixture; the subprocess integration test needs `llvm-symbolizer` on PATH or `PLANTY_TEST_SYMBOLIZER_IMAGE` pointing at a Docker image whose entrypoint is that binary.
