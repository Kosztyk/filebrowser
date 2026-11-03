#!/usr/bin/env python3
"""Isolated native upgrade, forced-restart resume, range and checksum test."""
import hashlib,json,os,resource,secrets,socket,subprocess,tempfile,time,urllib.parse,urllib.request
from pathlib import Path

def main():
 repo=Path(__file__).resolve().parents[1]
 binary=Path(os.environ.get('FILEBROWSER_TEST_BINARY',repo/'native-install/linux-amd64/filebrowser'))
 initializer=Path(os.environ.get('FILEBROWSER_BASELINE_BINARY',binary))
 size=int(os.environ.get('FILEBROWSER_TEST_BYTES',64*1024*1024+123));began=time.monotonic()
 with tempfile.TemporaryDirectory(prefix='filebrowser-batch1-') as tmp:
  tmp=Path(tmp);root=tmp/'files';root.mkdir();db=tmp/'test.db'
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}';username,password='batch1-test',secrets.token_urlsafe(24)
  for args in [['config','init','--root',str(root),'--disableExec=true'],['users','add',username,password,'--perm.execute=false']]:subprocess.run([str(initializer),'--database',str(db),*args],cwd=tmp,check=True,capture_output=True)
  log=(tmp/'server.log').open('w')
  def start():
   p=subprocess.Popen([str(binary),'--database',str(db),'--address','127.0.0.1','--port',str(port)],cwd=tmp,stdout=log,stderr=subprocess.STDOUT)
   for _ in range(100):
    try:
     with urllib.request.urlopen(base+'/health',timeout=1):return p
    except OSError:
     if p.poll() is not None:raise RuntimeError('Server failed: '+(tmp/'server.log').read_text())
     time.sleep(.1)
   raise RuntimeError('Server startup timeout')
  server=start()
  try:
   def request(path,method='GET',data=None,headers=None):return urllib.request.urlopen(urllib.request.Request(urllib.parse.urljoin(base,path),method=method,data=data,headers=headers or {}),timeout=120)
   with request('/api/login','POST',json.dumps({'username':username,'password':password}).encode(),{'Content-Type':'application/json'}) as r:token=r.read().decode()
   auth={'X-Auth':token,'Tus-Resumable':'1.0.0'};target=root/'upgrade-transfer.bin';target.write_bytes(b'original destination')
   with request('/api/tus/upgrade-transfer.bin?override=true','POST',b'',{**auth,'Upload-Length':str(size)}) as r:assert r.status==201;location=r.headers['Location']
   chunk=bytes(range(256))*32768;digest=hashlib.sha256();offset=0;restarted=False
   while offset<size:
    part=chunk[:min(len(chunk),size-offset)]
    with request(location,'PATCH',part,{**auth,'Content-Type':'application/offset+octet-stream','Upload-Offset':str(offset)}) as r:assert r.status==204;offset+=len(part);assert int(r.headers['Upload-Offset'])==offset
    digest.update(part)
    if offset==3*len(chunk):
     assert target.read_bytes()==b'original destination';server.kill();server.wait(timeout=10);server=start();restarted=True
     with request(location,'HEAD',headers=auth) as r:assert int(r.headers['Upload-Offset'])==offset
   for _ in range(300):
    with request(location,headers=auth) as r:info=json.load(r)
    if info['status']=='ready':break
    if info['status'] in ('failed','infected'):raise RuntimeError(str(info))
    time.sleep(.1)
   else:raise RuntimeError('Publication timeout')
   assert info['sha256']==digest.hexdigest() and info['scan']['status']=='disabled'
   with request('/api/resources/',headers=auth) as r:listing=json.load(r)
   assert '.filebrowser-staging' not in json.dumps(listing)
   with request('/api/raw/upgrade-transfer.bin',headers={**auth,'Range':'bytes=300-555'}) as r:assert r.status==206 and r.headers['Content-Range']==f'bytes 300-555/{size}' and r.read()==chunk[300:556]
   downloaded=hashlib.sha256();count=0
   with request('/api/raw/upgrade-transfer.bin',headers=auth) as r:
    while part:=r.read(1024*1024):downloaded.update(part);count+=len(part)
   assert count==size and downloaded.digest()==digest.digest()
   server.terminate();server.wait(timeout=10);hwm=resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss
   if hwm:assert hwm<512*1024, f'Peak backend process RSS too high: {hwm} KiB'
   print(json.dumps({'upload_bytes':size,'download_bytes':count,'forced_restart_recovery':restarted,'range':'pass','private_staging':'pass','scan_state':info['scan']['status'],'sha256':digest.hexdigest(),'backend_processes_peak_rss_kib':hwm or None,'elapsed_seconds':round(time.monotonic()-began,2)}),flush=True)
   if os.environ.get('PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH'):
    server=start();subprocess.run(['node','tests/batch1-app.mjs'],cwd=repo/'frontend',input=json.dumps({'base':base,'username':username,'password':password,'temporary':str(tmp)}),text=True,check=True)
  finally:
   server.terminate()
   try:server.wait(timeout=10)
   except subprocess.TimeoutExpired:server.kill();server.wait()
   log.close()
if __name__=='__main__':main()
