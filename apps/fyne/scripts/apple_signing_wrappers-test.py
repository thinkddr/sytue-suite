#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Exercise production wrapper arguments with synthetic native command boundaries."""
import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class Wrappers(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="owned-signing-wrappers-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.native = self.root / "native"
        self.native.mkdir()
        for name in ("xcrun", "codesign", "productsign", "productbuild", "security"):
            p = self.native / name
            p.write_text("#!/usr/bin/env python3\nimport json,sys\nprint(json.dumps(sys.argv[1:]))\n")
            p.chmod(0o700)
        helper = self.root / "helper.py"
        helper.write_text("import os\nprint(os.environ['OWN_FIXTURE'])\n")
        self.own = str(self.root / 'own space $value".keychain-db')
        self.env = dict(os.environ, SYTUE_APPLE_KEYCHAIN_HELPER=str(helper), OWN_FIXTURE=self.own,
                        FAKE_TOOL_DIR=str(self.native), SYTUE_APPLE_JOB_DIR=str(self.root))
        xcrun = (ROOT / ".github/actions/apple/xcrun-existing-only.sh").read_text()
        # Replace just the native exec boundary in a private test copy. Production
        # has no override or test flag that could redirect the native executables.
        xcrun = xcrun.replace("/usr/bin/xcrun", str(self.native / "xcrun"))
        (self.root / "xcrun").write_text(xcrun)
        (self.root / "xcrun").chmod(0o700)
        signing = (ROOT / ".github/actions/apple/sign-existing-only.sh").read_text()
        signing = signing.replace('"/usr/bin/$tool"', '"$FAKE_TOOL_DIR/$tool"')
        for name in ("codesign", "productsign", "productbuild"):
            (self.root / name).write_text(signing)
            (self.root / name).chmod(0o700)
        security = (ROOT / ".github/actions/apple/security-existing-only.sh").read_text()
        (self.root / "security").write_text(security.replace("/usr/bin/security", str(self.native / "security")))
        (self.root / "security").chmod(0o700)

    def run_wrapper(self, name, *args):
        return subprocess.run(["/bin/bash", str(self.root / name), *args], env=self.env,
                              capture_output=True, text=True)

    def test_direct_signers_only_use_own_keychain_and_preserve_arguments(self):
        for name in ("codesign", "productsign", "productbuild"):
            with self.subTest(tool=name):
                result = self.run_wrapper(name, "--sign", "existing identity", "a space.pkg")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result.stdout), ["--keychain", self.own, "--sign", "existing identity", "a space.pkg"])
                result = self.run_wrapper(name, "--sign", "existing", "--keychain", "/fixture/personal")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, "")

    def test_existing_own_scope_is_not_duplicated_and_inspection_is_preserved(self):
        result = self.run_wrapper("codesign", "-s", "existing", "--keychain", self.own, "app")
        self.assertEqual(json.loads(result.stdout).count("--keychain"), 1)
        result = self.run_wrapper("codesign", "--verify", "--strict", "app")
        self.assertEqual(json.loads(result.stdout), ["--verify", "--strict", "app"])

    def test_attached_and_combined_codesign_short_options_are_scoped(self):
        for arguments in (("-sexisting identity", "app"), ("-fs", "existing identity", "app"),
                          ("-fvsexisting identity", "app")):
            with self.subTest(arguments=arguments):
                result = self.run_wrapper("codesign", *arguments)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result.stdout), ["--keychain", self.own, *arguments])
                blocked = self.run_wrapper("codesign", *arguments, "--keychain", "/fixture/personal")
                self.assertEqual(blocked.returncode, 64)
                self.assertEqual(blocked.stdout, "")

    def test_xcode_removes_updates_rejects_registration_and_preserves_other_flags(self):
        result = self.run_wrapper("xcrun", "--sdk", "iphoneos", "xcodebuild", "-archive", "-allowProvisioningUpdates",
                                  "OTHER_CODE_SIGN_FLAGS=--timestamp --options runtime")
        self.assertEqual(result.returncode, 0, result.stderr)
        argv = json.loads(result.stdout)
        self.assertEqual(argv[:4], ["--sdk", "iphoneos", "xcodebuild", "-archive"])
        flags = [a for a in argv if a.startswith("OTHER_CODE_SIGN_FLAGS=")]
        self.assertEqual(len(flags), 1)
        # Xcode passes this setting through a shell. shlex alone does not undo
        # the shell escape for a literal dollar inside double quotes.
        shell = subprocess.run(["/bin/bash", "-c", "exec " + shlex.quote(str(self.native / "codesign")) + " " + flags[0].split("=", 1)[1]],
                               env=self.env, capture_output=True, text=True)
        self.assertEqual(shell.returncode, 0, shell.stderr)
        self.assertEqual(json.loads(shell.stdout), ["--keychain", self.own, "--timestamp", "--options", "runtime"])
        for flag in ("-allowProvisioningDeviceRegistration", "-allowProvisioningDeviceRegistration=YES"):
            blocked = self.run_wrapper("xcrun", "xcodebuild", flag)
            self.assertEqual(blocked.returncode, 64)
            self.assertEqual(blocked.stdout, "")
        blocked = self.run_wrapper("xcrun", "xcodebuild", "OTHER_CODE_SIGN_FLAGS=--keychain /fixture/personal")
        self.assertEqual(blocked.returncode, 64)
        self.assertEqual(blocked.stdout, "")

    def test_unsigned_xcrun_operations_pass_through(self):
        result = self.run_wrapper("xcrun", "stapler", "validate", "a space.dmg")
        self.assertEqual(json.loads(result.stdout), ["stapler", "validate", "a space.dmg"])

    def test_existing_sdk_queries_pass_through_without_a_tool(self):
        for flag in ("--show-sdk-path", "-show-sdk-path", "--show-sdk-version", "-show-sdk-version"):
            for prefix in (("--sdk", "iphoneos"), ("-sdk", "macosx")):
                with self.subTest(flag=flag, prefix=prefix):
                    result = self.run_wrapper("xcrun", *prefix, flag)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(json.loads(result.stdout), [*prefix, flag])
                    blocked = self.run_wrapper("xcrun", *prefix, flag, "codesign", "--sign", "existing")
                    self.assertEqual(blocked.returncode, 64)
                    self.assertEqual(blocked.stdout, "")

    def test_xcrun_signers_and_find_use_the_owned_wrappers(self):
        for name in ("codesign", "productsign", "productbuild"):
            with self.subTest(tool=name):
                wrapper = self.root / name
                for flag in ("--find", "-f"):
                    found = self.run_wrapper("xcrun", "--sdk", "macosx", flag, name)
                    self.assertEqual(found.returncode, 0, found.stderr)
                    self.assertEqual(found.stdout, str(wrapper) + "\n")
                arguments = ("--sign", "existing", "--", "a space.app")
                for prefix in ((), ("--sdk", "macosx"), ("--",)):
                    dispatch = self.run_wrapper("xcrun", *prefix, name, *arguments)
                    self.assertEqual(dispatch.returncode, 0, dispatch.stderr)
                    self.assertEqual(json.loads(dispatch.stdout), [*prefix, str(wrapper), *arguments])
                # Follow exactly the executable selected by xcrun/find.
                signed = subprocess.run(["/bin/bash", str(wrapper), *arguments], env=self.env,
                                        capture_output=True, text=True)
                self.assertEqual(signed.returncode, 0, signed.stderr)
                self.assertEqual(json.loads(signed.stdout), ["--keychain", self.own, *arguments])
                blocked = subprocess.run(["/bin/bash", str(wrapper), "--sign", "existing",
                                          "--keychain", "/fixture/personal", "--", "app"],
                                         env=self.env, capture_output=True, text=True)
                self.assertEqual(blocked.returncode, 64)
                self.assertEqual(blocked.stdout, "")

    def test_xcode_scope_precedes_an_existing_separator(self):
        result = self.run_wrapper("xcrun", "xcodebuild", "OTHER_CODE_SIGN_FLAGS=--timestamp --")
        self.assertEqual(result.returncode, 0, result.stderr)
        flags = json.loads(result.stdout)[1].split("=", 1)[1]
        shell = subprocess.run(["/bin/bash", "-c", "exec " + shlex.quote(str(self.native / "codesign")) + " " + flags],
                               env=self.env, capture_output=True, text=True)
        self.assertEqual(shell.returncode, 0, shell.stderr)
        self.assertEqual(json.loads(shell.stdout), ["--keychain", self.own, "--timestamp", "--"])
        blocked = self.run_wrapper("xcrun", "--find", "xcodebuild")
        self.assertEqual(blocked.returncode, 64)
        self.assertEqual(blocked.stdout, "")


    def test_fyne_unscoped_lookup_is_bound_to_owned_keychain_and_personal_lookup_rejected(self):
        result = self.run_wrapper("security", "find-certificate", "-c", "Apple Distribution: existing", "-p")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), ["find-certificate", "-c", "Apple Distribution: existing", "-p", self.own])
        # Exact shapes used by the repo's identity/profile readers. Fyne Tools
        # itself uses only the unscoped find-certificate lookup above.
        for arguments in (("find-identity", "-v", "-p", "codesigning"),
                          ("find-identity", "-p", "codesigning", self.own),
                          ("find-identity", "-v", self.own),
                          ("find-certificate", "-a", "-Z", "-p", self.own),
                          ("show-keychain-info", self.own)):
            with self.subTest(reader=arguments[0], arguments=arguments[1:]):
                result = self.run_wrapper("security", *arguments)
                self.assertEqual(result.returncode, 0, result.stderr)
                expected = list(arguments) if arguments[-1] == self.own else [*arguments, self.own]
                self.assertEqual(json.loads(result.stdout), expected)
        for args in (("find-certificate", "-p", "/fixture/personal.keychain-db"),
                     ("list-keychains", "-d", "user", "-s", self.own), ("default-keychain", "-s", self.own),
                     ("import", "/fixture/private.p12"), ("add-trusted-cert", "/fixture/cert"),
                     ("cms", "-D", "-i", "/fixture/profile.mobileprovision")):
            with self.subTest(command=args[0]):
                blocked = self.run_wrapper("security", *args)
                self.assertEqual(blocked.returncode, 64)
                self.assertEqual(blocked.stdout, "")

    def test_native_policy_read_keeps_scope_and_rejects_anchor_override(self):
        cert = self.root / "leaf.pem"
        cert.write_text("public certificate fixture")
        result = self.run_wrapper("security", "verify-cert", "-L", "-p", "codeSign", "-c", str(cert), "-k", self.own)
        self.assertEqual(result.returncode, 0, result.stderr)
        for scope in ((), ("-k", "/fixture/personal"), ("-k", self.own, "-r", "/fixture/alternate-root")):
            blocked = self.run_wrapper("security", "verify-cert", "-L", "-p", "codeSign", "-c", str(cert), *scope)
            self.assertEqual(blocked.returncode, 64)
            self.assertEqual(blocked.stdout, "")


if __name__ == "__main__":
    unittest.main(verbosity=2)
