import subprocess,json,time,os,sys,pathlib,hashlib,argparse
root=pathlib.Path(__file__).resolve().parents[3];ev=pathlib.Path(__file__).resolve().parent
parser=argparse.ArgumentParser(description="Run one command and record its exit code, duration and log digest in this evidence directory.",epilog="Example: python3 check.py scoped go test -count=1 -run ContinualOwnership -v ./experiment")
parser.add_argument("name",help="Unused filename prefix for NAME.log and NAME.json; existing evidence is never overwritten.")
parser.add_argument("command",nargs=argparse.REMAINDER,help="Command and arguments, forwarded verbatim. The default working directory is the repository root; COIMNET_CHECK_CWD overrides it.")
if len(sys.argv)==1:
 parser.print_help();sys.exit(0)
args=parser.parse_args()
if not args.command:parser.error("a command is required")
if pathlib.Path(args.name).name!=args.name or args.name in {".",".."}:parser.error("name must be a filename prefix")
name=args.name;cmd=args.command;t=time.monotonic()
with (ev/(name+'.log')).open('xb') as log:
 p=subprocess.run(cmd,cwd=os.environ.get('COIMNET_CHECK_CWD',root),stdout=log,stderr=subprocess.STDOUT)
meta={'command':cmd,'cwd':os.environ.get('COIMNET_CHECK_CWD',str(root)),'exit_code':p.returncode,'seconds':time.monotonic()-t,'log':name+'.log','log_sha256':hashlib.sha256((ev/(name+'.log')).read_bytes()).hexdigest()}
(ev/(name+'.json')).open('x').write(json.dumps(meta,indent=2)+'\n');print(json.dumps(meta));sys.exit(p.returncode)
