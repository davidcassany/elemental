#!/usr/bin/env python3

import importlib.util
import pathlib
import unittest

import yaml


def load_module():
    path = pathlib.Path(__file__).with_name("patch-core-manifests.py")
    spec = importlib.util.spec_from_file_location("patch_core_manifests", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class PatchReleaseManifestTest(unittest.TestCase):
    def test_patches_base_and_iso_and_removes_obsolete_elemental3ctl_sysext(self):
        module = load_module()
        data = """\
schema: v0
components:
  operatingSystem:
    image:
      base: registry.example/base:old
      iso: registry.example/iso:old
  systemd:
    extensions:
    - name: elemental3ctl
      image: registry.example/elemental3ctl:old
      required: true
    - name: qemu-guest-agent
      image: registry.example/qga:1
"""

        patched, removed = module.patch_release_manifest(
            data,
            "index.docker.io/ravan/base:fixed",
            "index.docker.io/ravan/iso:fixed",
        )

        manifest = yaml.safe_load(patched)
        image = manifest["components"]["operatingSystem"]["image"]
        self.assertEqual(image["base"], "index.docker.io/ravan/base:fixed")
        self.assertEqual(image["iso"], "index.docker.io/ravan/iso:fixed")
        self.assertTrue(removed)
        self.assertEqual(
            manifest["components"]["systemd"]["extensions"],
            [{"name": "qemu-guest-agent", "image": "registry.example/qga:1"}],
        )


if __name__ == "__main__":
    unittest.main()
