import importlib.util
import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('release_policy',pathlib.Path(__file__).with_name('release.py'))
policy=importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)

class ReleaseGates(unittest.TestCase):
    def test_no_publish_from_wrong_checkout(self):
        with patch.object(policy,'version_at',return_value='1.0.0'), patch.object(policy,'run',return_value='other'), patch.object(policy,'api') as api:
            with self.assertRaisesRegex(RuntimeError,'checkout'):
                policy.publish('approved','1.0.0','1')
            api.assert_not_called()

    def test_no_publish_incomplete_distribution(self):
        with tempfile.TemporaryDirectory() as root:
            directory=pathlib.Path(root)/'build/native/1.0.0'
            directory.mkdir(parents=True)
            (directory/'checksums.json').write_text(json.dumps({'version':'1.0.0','source_commit':'approved','channel':'stable','files':{}}))
            import os
            old=os.getcwd()
            try:
                os.chdir(root)
                with patch.object(policy,'version_at',return_value='1.0.0'),patch.object(policy,'run',return_value='approved'),patch.object(policy,'api') as api:
                    with self.assertRaisesRegex(RuntimeError,'Incomplete'):
                        policy.publish('approved','1.0.0','1')
                    api.assert_not_called()
            finally:
                os.chdir(old)

    def test_pending_release_uses_merge_sha_not_latest_main(self):
        with patch.object(policy,'baseline',return_value='v0.9.0'),patch.object(policy,'pending',return_value=[{'merge_commit_sha':'fixed','number':7}]),patch.object(policy,'version_at',return_value='1.0.0'),patch.object(policy,'run',return_value=json.dumps({'version':'1.0.0'})),patch.object(policy,'output') as output:
            policy.plan()
            self.assertEqual(output.call_args.args[0],{'publish':True,'prepare':False,'sha':'fixed','version':'1.0.0','pr':7})

    def test_understated_release_rejected(self):
        with patch.object(policy,'baseline',return_value='v0.9.0'),patch.object(policy,'pending',return_value=[{'merge_commit_sha':'fixed','number':7}]),patch.object(policy,'version_at',return_value='0.10.0'),patch.object(policy,'run',return_value=json.dumps({'version':'1.0.0'})):
            with self.assertRaisesRegex(RuntimeError,'understates'):
                policy.plan()

    def test_manifest_mismatch_rejected(self):
        with patch.object(policy,'run',side_effect=['const Version = "1.0.0"','{".":"0.9.0"}']):
            with self.assertRaisesRegex(RuntimeError,'manifest differ'):
                policy.version_at('fixed')

if __name__=='__main__':unittest.main()
