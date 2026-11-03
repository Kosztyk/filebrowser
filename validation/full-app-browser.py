#!/usr/bin/env python3
"""Run the enhanced UI against an isolated native instance and disposable files."""
import json,os,secrets,shutil,socket,subprocess,tempfile,time,urllib.request
from pathlib import Path
repo=Path(__file__).resolve().parents[1]
binary=Path(os.environ.get('FILEBROWSER_TEST_BINARY',repo/'native-install/linux-amd64/filebrowser'))
with tempfile.TemporaryDirectory(prefix='filebrowser-full-ui-') as tmp:
 tmp=Path(tmp);root=tmp/'data';root.mkdir();db=tmp/'test.db'
 for i in range(501):(root/f'file-{i}.txt').write_text('fixture content '+str(i))
 with (root/'large.csv').open('w') as f:
  f.write('name,value\n')
  for i in range(190000):f.write(f'record-{i},some fixture value {i}\n')
 (root/'large.log').write_text('Bounded log fixture entry\n'*240000)
 shutil.copy2(repo/'validation/fixtures/tiny.tiff',root/'image.tiff')
 layout=root/'layout';layout.mkdir()
 for i in range(29):(layout/f'folder-{i:02}').mkdir()
 for i in range(15):
  name=layout/f'file-{i:02}.txt';name.write_text('fixture '+str(i)+'x'*(i*10));os.utime(name,(1700000000+i,1700000000+i))
 (layout/'drag-source.txt').write_text('move this fixture')
 (root/'folder').mkdir();(root/'folder/one.txt').write_text('one');(root/'folder/two.txt').write_text('two')
 username,password='ui-test',secrets.token_urlsafe(24)
 for args in [['config','init','--root',str(root),'--disableExec=true'],['users','add',username,password,'--perm.admin=true','--perm.execute=false']]:subprocess.run([str(binary),'--database',str(db),*args],cwd=tmp,check=True,capture_output=True)
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 base=f'http://127.0.0.1:{port}'
 with (tmp/'server.log').open('w') as log:
  server=subprocess.Popen([str(binary),'--database',str(db),'--address','127.0.0.1','--port',str(port)],cwd=tmp,stdout=log,stderr=subprocess.STDOUT)
  try:
   for _ in range(100):
    try:urllib.request.urlopen(base+'/health',timeout=1);break
    except OSError:
     if server.poll() is not None:raise RuntimeError((tmp/'server.log').read_text())
     time.sleep(.1)
   else:raise RuntimeError('Native server health check timed out')
   subprocess.run(['node',str(repo/'validation/full-app-browser.mjs')],input=json.dumps({'base':base,'username':username,'password':password}),text=True,check=True,timeout=240,cwd=repo)
  finally:
   server.terminate()
   try:server.wait(10)
   except subprocess.TimeoutExpired:server.kill();server.wait()
