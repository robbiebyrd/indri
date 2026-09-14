// The plugin this exercises is the only thing standing between a clean checkout
// and an iOS build that dies inside a vendored dependency, and it runs where
// nobody watches it: inside `pod install`, against files that are gitignored.
// So the two fixtures below are copied VERBATIM out of the real headers —
// fmt 11.0.2 as React Native 0.79.6 vendors it, and fmt 11.1.0 as upstream
// fixed it. The interesting property is that their `FMT_USE_CONSTEVAL` chains
// are byte-identical: only the format-string constructor changed. A plugin that
// keyed on the chain alone would happily patch a fmt that no longer needs it,
// which is the thing criterion 4 is about.
import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import plugin from "./with-fmt-consteval-fix.js";

const {MARKER, patchBaseHeader, patchPodfile, applyToPods, PODFILE_HOOK} = plugin;

/**
 * The `FMT_USE_CONSTEVAL` chain. Identical in fmt 11.0.2 and 11.1.0, which is
 * why it can be shared by both fixtures.
 */
const CONSTEVAL_CHAIN = `
// Detect consteval, C++20 constexpr extensions and std::is_constant_evaluated.
#if !defined(__cpp_lib_is_constant_evaluated)
#  define FMT_USE_CONSTEVAL 0
#elif FMT_CPLUSPLUS < 201709L
#  define FMT_USE_CONSTEVAL 0
#elif FMT_GLIBCXX_RELEASE && FMT_GLIBCXX_RELEASE < 10
#  define FMT_USE_CONSTEVAL 0
#elif FMT_LIBCPP_VERSION && FMT_LIBCPP_VERSION < 10000
#  define FMT_USE_CONSTEVAL 0
#elif defined(__apple_build_version__) && __apple_build_version__ < 14000029L
#  define FMT_USE_CONSTEVAL 0  // consteval is broken in Apple clang < 14.
#elif FMT_MSC_VERSION && FMT_MSC_VERSION < 1929
#  define FMT_USE_CONSTEVAL 0  // consteval is broken in MSVC VS2019 < 16.10.
#elif defined(__cpp_consteval)
#  define FMT_USE_CONSTEVAL 1
#elif FMT_GCC_VERSION >= 1002 || FMT_CLANG_VERSION >= 1101
#  define FMT_USE_CONSTEVAL 1
#else
#  define FMT_USE_CONSTEVAL 0
#endif
#if FMT_USE_CONSTEVAL
#  define FMT_CONSTEVAL consteval
#  define FMT_CONSTEXPR20 constexpr
#else
#  define FMT_CONSTEVAL
#  define FMT_CONSTEXPR20
#endif
`;

/** fmt 11.0.2: one constructor, consteval, and it takes compile strings. */
const FMT_11_0_2 = `#define FMT_VERSION 110002
${CONSTEVAL_CHAIN}
/// A compile-time format string.
template <typename Char, typename... Args> class basic_format_string {
 private:
  basic_string_view<Char> str_;

 public:
  template <
      typename S,
      FMT_ENABLE_IF(
          std::is_convertible<const S&, basic_string_view<Char>>::value ||
          (detail::is_compile_string<S>::value &&
           std::is_constructible<basic_string_view<Char>, const S&>::value))>
  FMT_CONSTEVAL FMT_ALWAYS_INLINE basic_format_string(const S& s) : str_(s) {
#if FMT_USE_CONSTEVAL
    if constexpr (detail::count_named_args<Args...>() ==
                  detail::count_statically_named_args<Args...>()) {
      using checker =
          detail::format_string_checker<Char, remove_cvref_t<Args>...>;
      detail::parse_format_string<true>(str_, checker(s));
    }
#else
    detail::check_format_string<Args...>(s);
#endif
  }
};
`;

/**
 * fmt 11.1.0: `basic_format_string` is gone, and the compile-string overload of
 * its replacement is pointedly NOT consteval. That is the upstream fix.
 */
const FMT_11_1_0 = `#define FMT_VERSION 110100
${CONSTEVAL_CHAIN}
/// A compile-time format string.
template <typename... T> struct fstring {
 public:
  template <size_t N>
  FMT_CONSTEVAL FMT_ALWAYS_INLINE fstring(const char (&s)[N]) : str(s, N - 1) {}
  template <typename S,
            FMT_ENABLE_IF(std::is_base_of<detail::compile_string, S>::value&&
                              std::is_same<typename S::char_type, char>::value)>
  FMT_ALWAYS_INLINE fstring(const S&) : str(S()) {
    FMT_CONSTEXPR auto sv = string_view(S());
  }
};

template <typename... T> using format_string = typename fstring<T...>::t;
`;

/** A generated Podfile, trimmed to the parts the plugin cares about. */
const PODFILE = `platform :ios, '15.1'

target 'indriclient' do
  use_expo_modules!

  post_install do |installer|
    react_native_post_install(
      installer,
      config[:reactNativePath],
    )
  end
end
`;

/** Run \`body\`, returning everything it wrote to stdout/stderr via console. */
function captureConsole(body: () => void): string[] {
    const lines: string[] = [];
    const sink = (...args: unknown[]) => void lines.push(args.join(" "));
    const {log, warn, error} = console;
    Object.assign(console, {log: sink, warn: sink, error: sink});
    try {
        body();
    } finally {
        Object.assign(console, {log, warn, error});
    }
    return lines;
}

/** A throwaway CocoaPods sandbox holding a read-only fmt header, as pod install leaves it. */
function makePodsSandbox(header: string | null): string {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "indri-fmt-"));
    if (header !== null) {
        const dir = path.join(root, "fmt", "include", "fmt");
        fs.mkdirSync(dir, {recursive: true});
        const file = path.join(dir, "base.h");
        fs.writeFileSync(file, header);
        fs.chmodSync(file, 0o444);
    }
    return root;
}

function headerIn(root: string): string {
    return path.join(root, "fmt", "include", "fmt", "base.h");
}

test("patches fmt 11.0.2 by forcing the first branch of the consteval chain", () => {
    const {status, contents} = patchBaseHeader(FMT_11_0_2);

    assert.equal(status, "patched");
    // The chain must no longer be able to reach a branch that defines 1: the
    // rewritten `#if 1` short-circuits every `#elif` after it.
    assert.ok(!contents.includes("#if !defined(__cpp_lib_is_constant_evaluated)"));
    assert.match(contents, /^#if 1 {2}\/\/ indri-fmt-consteval-fix:/m);
    // ...and the branch it now takes is still the one that switches consteval OFF.
    assert.match(
        contents,
        /^#if 1 {2}\/\/ indri-fmt-consteval-fix:[^\n]*\n# {2}define FMT_USE_CONSTEVAL 0$/m,
    );

    // Nothing else may move. A patch that edits more of a vendored header than
    // it claims to is a patch nobody can review.
    const before = FMT_11_0_2.split("\n");
    const after = contents.split("\n");
    assert.equal(after.length, before.length);
    const changed = before.map((line, i) => i).filter((i) => before[i] !== after[i]);
    assert.deepEqual(changed.map((i) => before[i]), [
        "#if !defined(__cpp_lib_is_constant_evaluated)",
    ]);
});

test("is a no-op on fmt 11.1, which carries the upstream fix", () => {
    // 11.1 still has the identical FMT_USE_CONSTEVAL chain, so this only passes
    // because the plugin looks for the consteval constructor, not the chain.
    assert.ok(FMT_11_1_0.includes("#if !defined(__cpp_lib_is_constant_evaluated)"));

    const {status, contents} = patchBaseHeader(FMT_11_1_0);

    assert.equal(status, "not-needed");
    assert.equal(contents, FMT_11_1_0);
});

test("re-patching a patched header changes nothing", () => {
    const once = patchBaseHeader(FMT_11_0_2).contents;
    const twice = patchBaseHeader(once);

    assert.equal(twice.status, "already-patched");
    assert.equal(twice.contents, once);
});

test("refuses to stay silent when fmt is broken but unrecognisable", () => {
    // The consteval constructor is still there, so the build WILL fail — but the
    // chain we know how to rewrite is not. Reporting "nothing to do" here would
    // hand the next person the same unexplained compiler error this plugin exists
    // to prevent, so it has to raise instead.
    const mangled = FMT_11_0_2.replace(
        "#if !defined(__cpp_lib_is_constant_evaluated)",
        "#if !FMT_SOMETHING_ELSE",
    );

    assert.throws(
        () => patchBaseHeader(mangled),
        (err: Error) => err.message.includes(MARKER) && /FMT_USE_CONSTEVAL chain/.test(err.message),
    );
});

test("restores write permission on the read-only header CocoaPods installs", () => {
    const root = makePodsSandbox(FMT_11_0_2);
    const file = headerIn(root);

    // Guard the guard: if the fixture were writable this test would prove nothing.
    assert.throws(() => fs.accessSync(file, fs.constants.W_OK));

    const log = captureConsole(() => assert.equal(applyToPods(root), "patched"));

    assert.deepEqual(log, [`[${MARKER}] ${file}: patched`]);
    assert.ok(fs.readFileSync(file, "utf8").includes(MARKER));
    fs.rmSync(root, {recursive: true, force: true});
});

test("a second pod install over an already-patched sandbox is a no-op", () => {
    const root = makePodsSandbox(FMT_11_0_2);
    const file = headerIn(root);

    captureConsole(() => applyToPods(root));
    const afterFirst = fs.readFileSync(file, "utf8");
    const log = captureConsole(() => assert.equal(applyToPods(root), "already-patched"));

    assert.deepEqual(log, [`[${MARKER}] ${file}: already-patched`]);
    assert.equal(fs.readFileSync(file, "utf8"), afterFirst);
    fs.rmSync(root, {recursive: true, force: true});
});

test("says so, without failing, when the sandbox has no fmt pod", () => {
    const root = makePodsSandbox(null);

    const log = captureConsole(() => assert.equal(applyToPods(root), "missing"));

    assert.deepEqual(log, [`[${MARKER}] no fmt pod at ${headerIn(root)} — nothing patched.`]);
    fs.rmSync(root, {recursive: true, force: true});
});

test("hooks the patch into the generated Podfile's post_install block", () => {
    const patched = patchPodfile(PODFILE);

    assert.ok(patched.includes(PODFILE_HOOK));
    // It must land INSIDE post_install, where `installer` is in scope, and before
    // the block closes.
    const start = patched.indexOf("post_install do |installer|");
    const hook = patched.indexOf(PODFILE_HOOK);
    assert.ok(start !== -1 && hook > start);
    assert.ok(hook < patched.indexOf("react_native_post_install"));
    // It has to reach this very file, and hand it the pod sandbox.
    assert.ok(patched.includes("plugins', 'with-fmt-consteval-fix.js'"));
    assert.ok(patched.includes("installer.sandbox.root.to_s"));
    // A patch that fails must stop the build rather than warn into the scrollback.
    assert.ok(patched.includes("raise"));
});

test("re-running prebuild over an existing Podfile does not stack hooks", () => {
    const once = patchPodfile(PODFILE);

    assert.equal(patchPodfile(once), once);
});

test("refuses a Podfile with no post_install block to hook into", () => {
    const noHook = PODFILE.replace(/  post_install do \|installer\|[\s\S]*?\n  end\n/, "");

    assert.ok(!noHook.includes("post_install"));
    assert.throws(
        () => patchPodfile(noHook),
        (err: Error) => err.message.includes(MARKER) && err.message.includes("post_install"),
    );
});

test("app.json registers the plugin, or prebuild never runs it", () => {
    const appJson = JSON.parse(
        fs.readFileSync(path.join(import.meta.dirname, "..", "app.json"), "utf8"),
    );
    const names = appJson.expo.plugins.map((p: string | [string, unknown]) =>
        Array.isArray(p) ? p[0] : p,
    );

    assert.ok(names.includes("./plugins/with-fmt-consteval-fix"));
});
