__attribute__((noinline)) int native_crash_site(int value) {
    return value + 1;
}

int main(void) {
    return native_crash_site(40);
}
