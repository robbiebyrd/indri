const fs = require('fs');
const path = require('path');

/**
 * Make React Native's vendored `fmt` compile under the clang in Xcode 26.
 *
 * THE BUG. React Native 0.79.6 vendors fmt 11.0.2. In that version the only
 * constructor of `basic_format_string` is `FMT_CONSTEVAL`, and it is the one
 * that accepts compile-time format strings (`FMT_STRING`, `is_compile_string`).
 * The clang shipped in Xcode 26 rejects that path — "call to consteval function
 * ... is not a constant expression" — so *no* native iOS build of this app
 * compiles. The failure is inside React Native's own pod and has nothing to do
 * with any feature of this app.
 *
 * WHY NOT A BUILD SETTING. `fmt/base.h` picks `FMT_USE_CONSTEVAL` with a bare
 * `#if`/`#elif` chain and no `#ifndef` around it, so a `-DFMT_USE_CONSTEVAL=0`
 * supplied from `OTHER_CPLUSPLUSFLAGS` is simply overwritten by the header. The
 * only lever is the header text itself, hence a source patch.
 *
 * WHAT THE PATCH DOES. It forces the first branch of that chain to be taken.
 * That branch defines `FMT_USE_CONSTEVAL 0`, which leaves `FMT_CONSTEVAL`
 * expanding to nothing — exactly what fmt itself does for the toolchains it
 * already knows are broken (see the "consteval is broken in Apple clang < 14"
 * branch a few lines below the one we rewrite).
 *
 * WHEN IT RETIRES ITSELF. fmt 11.1 is the upstream fix: `basic_format_string`
 * was replaced by `fstring`, whose compile-string overload is deliberately NOT
 * `FMT_CONSTEVAL` any more. So instead of matching a version string — a second
 * thing to keep in step with reality — this plugin looks for the broken shape
 * itself (see `needsPatch`) and does nothing once React Native vendors fmt 11.1
 * or newer. Nobody has to remember to delete it.
 *
 * WHY IT IS A CONFIG PLUGIN. The patch has to run against `ios/Pods`, which
 * only exists after `pod install`. The natural home is the Podfile's
 * `post_install` hook — but `ios/` is generated output, is gitignored, and is
 * rewritten by every `expo prebuild`, so a patch parked there silently vanishes
 * and the next person meets an unexplained compiler error in a vendored
 * dependency. So this file does two jobs:
 *
 *   1. As an Expo config plugin (the default export) it re-injects a
 *      `post_install` hook into the freshly generated Podfile on every prebuild.
 *   2. As a CLI (`node plugins/with-fmt-consteval-fix.js <pods-root>`) it is
 *      what that hook calls. The patching logic therefore lives here, in one
 *      place, in a language this repo can unit-test — not in generated Ruby.
 */

/**
 * Stamped into both the Podfile and the patched header. Its presence is how
 * both patches detect their own previous work, which is what makes repeated
 * `expo prebuild` and `pod install` runs no-ops rather than double-patches.
 */
const MARKER = 'indri-fmt-consteval-fix';

/** Path of the header to patch, relative to the CocoaPods sandbox root. */
const FMT_BASE_HEADER = path.join('fmt', 'include', 'fmt', 'base.h');

/**
 * Head of the `FMT_USE_CONSTEVAL` chain in fmt 11.0.x/11.1, matched together
 * with the line it guards so we cannot rewrite some other `#if` that happens to
 * test the same macro.
 */
const CONSTEVAL_CHAIN_HEAD =
    '#if !defined(__cpp_lib_is_constant_evaluated)\n#  define FMT_USE_CONSTEVAL 0\n';

const PATCHED_CHAIN_HEAD =
    `#if 1  // ${MARKER}: Xcode 26 clang rejects fmt's consteval FMT_STRING path\n` +
    '#  define FMT_USE_CONSTEVAL 0\n';

/**
 * The broken shape, as it appears in fmt 11.0.x: a `basic_format_string` whose
 * constructor is `FMT_CONSTEVAL`. fmt 11.1 renamed the type to `fstring` and
 * dropped `FMT_CONSTEVAL` from its compile-string overload, so this matches the
 * versions that need the patch and nothing later.
 */
const VULNERABLE_CONSTEVAL_CTOR = /FMT_CONSTEVAL[^\n]*\bbasic_format_string\s*\(/;

/**
 * Decide what, if anything, to do to a copy of `fmt/base.h`.
 *
 * Returns one of:
 *   `already-patched` — we have been here before; `contents` is unchanged.
 *   `not-needed`      — fmt carries the upstream fix; `contents` is unchanged.
 *   `patched`         — `contents` is the rewritten header.
 *
 * Throws when the header still has the broken constructor but not the chain we
 * know how to rewrite. Returning "nothing to do" there would reproduce the
 * exact failure this plugin exists to prevent: a build that breaks much later,
 * somewhere else, for no stated reason.
 */
function patchBaseHeader(contents) {
    if (contents.includes(MARKER)) {
        return {status: 'already-patched', contents};
    }
    if (!VULNERABLE_CONSTEVAL_CTOR.test(contents)) {
        return {status: 'not-needed', contents};
    }
    if (!contents.includes(CONSTEVAL_CHAIN_HEAD)) {
        throw new Error(
            `${MARKER}: fmt/base.h still routes compile-time format strings through a ` +
            'consteval constructor, but its FMT_USE_CONSTEVAL chain does not look the way ' +
            'this patch expects, so it was not patched. Update ' +
            'client/plugins/with-fmt-consteval-fix.js against the vendored fmt.'
        );
    }
    return {
        status: 'patched',
        contents: contents.replace(CONSTEVAL_CHAIN_HEAD, PATCHED_CHAIN_HEAD),
    };
}

/**
 * Apply `patchBaseHeader` to the fmt pod inside a CocoaPods sandbox
 * (`installer.sandbox.root`, i.e. `ios/Pods`).
 *
 * `Pods/Headers/{Public,Private}/fmt/fmt/base.h` are symlinks to this one file,
 * so patching it once is enough to change what every target compiles against.
 */
function applyToPods(podsRoot) {
    const header = path.join(podsRoot, FMT_BASE_HEADER);
    if (!fs.existsSync(header)) {
        // Not fatal: a project that does not pull in fmt has nothing to patch.
        // Loud, because if fmt IS in the build and merely moved, the build will
        // fail later with a message that does not mention fmt at all.
        console.warn(`[${MARKER}] no fmt pod at ${header} — nothing patched.`);
        return 'missing';
    }

    const {status, contents} = patchBaseHeader(fs.readFileSync(header, 'utf8'));
    if (status === 'patched') {
        // CocoaPods installs pod sources read-only (0444), so writing without
        // this first fails with EACCES — and does so from inside `pod install`,
        // where the cause is far from obvious. We leave the file writable: the
        // next `pod install` re-extracts a pristine read-only copy anyway.
        fs.chmodSync(header, 0o644);
        fs.writeFileSync(header, contents);
    }
    console.log(`[${MARKER}] ${header}: ${status}`);
    return status;
}

/**
 * The Ruby we add to the generated Podfile. It runs this same file as a CLI, so
 * there is only ever one copy of the patching logic.
 *
 * `__dir__` is `client/ios`, which puts the script at `client/plugins/...`.
 * Shelling out to `node` is safe here: the generated Podfile already calls
 * `node --print` on its very first line to locate the Expo and React Native
 * pod scripts, so a Podfile that is being evaluated at all has node on PATH.
 *
 * A failure raises rather than warns. A skipped patch means a compiler error
 * later, in a vendored dependency, with nothing pointing back here.
 */
const PODFILE_HOOK = [
    `    # ${MARKER} — see ../plugins/with-fmt-consteval-fix.js for the why.`,
    '    # Injected on every `expo prebuild`, because this Podfile is generated',
    '    # output and is not tracked by git.',
    "    fmt_fix = File.join(__dir__, '..', 'plugins', 'with-fmt-consteval-fix.js')",
    "    unless system('node', fmt_fix, installer.sandbox.root.to_s)",
    `      raise '${MARKER}: failed to patch fmt; iOS builds will not compile. ` +
        "See client/plugins/with-fmt-consteval-fix.js'",
    '    end',
].join('\n');

/** Where we splice the hook in. Expo's bare template always opens this block. */
const POST_INSTALL_OPEN = 'post_install do |installer|\n';

/**
 * Add the hook to a generated Podfile, or leave it alone if it is already
 * there — `expo prebuild` without `--clean` reuses the existing Podfile, so
 * this runs against our own output more often than not.
 */
function patchPodfile(contents) {
    if (contents.includes(MARKER)) return contents;
    if (!contents.includes(POST_INSTALL_OPEN)) {
        throw new Error(
            `${MARKER}: the generated Podfile has no \`${POST_INSTALL_OPEN.trim()}\` block ` +
            'to hook into, so the fmt patch was not installed and the iOS build would fail ' +
            'in a vendored dependency. Update client/plugins/with-fmt-consteval-fix.js.'
        );
    }
    return contents.replace(
        POST_INSTALL_OPEN,
        `${POST_INSTALL_OPEN}${PODFILE_HOOK}\n\n`
    );
}

/** The Expo config plugin. Registered from `app.json`. */
function withFmtConstevalFix(config) {
    // Required lazily so the CLI half of this file does not need Expo loaded.
    const {withPodfile} = require('expo/config-plugins');
    return withPodfile(config, (podfileConfig) => {
        podfileConfig.modResults.contents = patchPodfile(
            podfileConfig.modResults.contents
        );
        return podfileConfig;
    });
}

module.exports = withFmtConstevalFix;
module.exports.default = withFmtConstevalFix;
module.exports.MARKER = MARKER;
module.exports.FMT_BASE_HEADER = FMT_BASE_HEADER;
module.exports.PODFILE_HOOK = PODFILE_HOOK;
module.exports.patchBaseHeader = patchBaseHeader;
module.exports.patchPodfile = patchPodfile;
module.exports.applyToPods = applyToPods;

if (require.main === module) {
    const podsRoot = process.argv[2];
    if (!podsRoot) {
        console.error(
            'usage: node plugins/with-fmt-consteval-fix.js <cocoapods-sandbox-root>'
        );
        process.exit(1);
    }
    try {
        applyToPods(podsRoot);
    } catch (err) {
        console.error(`[${MARKER}] ${err.message}`);
        process.exit(1);
    }
}
