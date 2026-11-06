"""Production binary with Git as the only executable discoverable on PATH."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

binary=Path(os.environ['RUM_BINARY']).resolve()
git=shutil.which('git')
assert git,'Git is required for this acceptance test'
with tempfile.TemporaryDirectory(prefix='rum-portable-',dir='/private/tmp') as folder:
    root=Path(folder);home=root/'device';commands=root/'commands';commands.mkdir()
    (commands/'git').symlink_to(git)
    configuration=root/'gitconfig';configuration.write_text('[user]\n name = Portable User\n email = portable@example.test\n')
    env=dict(os.environ,PATH=str(commands),GIT_CONFIG_GLOBAL=str(configuration),GIT_CONFIG_NOSYSTEM='1')
    code=root/'existing';code.mkdir();(code/'source.txt').write_text('retained source\n')
    subprocess.run([git,'init','-q','--initial-branch=main',str(code)],env=env,check=True)
    def call(*args):
        result=subprocess.run([str(binary),'--home',str(home),*map(str,args)],env=env,text=True,capture_output=True,check=True,timeout=20)
        return json.loads(result.stdout)['result']
    record=call('workspace-adopt','--path',code,'--date','2026-11-06')
    assert (code/'code/source.txt').read_text()=='retained source\n'
    assert call('doctor','--project',record['id'])['ok']
    call('project-identity','--project',record['id'],'--name','Meta User','--email','meta@example.test')
    info=call('explain');assert Path(info['manual']).is_file()
    assert call('platform','--project',record['id'],'--operation','check')['ok']
    assert not shutil.which('python3',path=env['PATH'])
    assert not shutil.which('go',path=env['PATH'])
print('PASS: production workspace adoption, real Git configuration, health, native workflow and offline manual; only Git on PATH')
