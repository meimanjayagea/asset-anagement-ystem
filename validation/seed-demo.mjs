const execute=process.argv.includes("--execute");
const confirmation=process.env.DEMO_SEED_CONFIRM;
if(!execute){
 console.log(JSON.stringify({mode:"dry-run",writes:0,branch:"DEMO",assets:26,users:9,photos:42,reset:false,note:"Explicit confirmation, org code and admin employee credentials are required."}));
 process.exit(0);
}
if(confirmation!=="SEED_DEMO_KEEP_EXISTING")throw new Error("DEMO_SEED_CONFIRM=SEED_DEMO_KEEP_EXISTING required");
const base=new URL(process.env.ASSETFLOW_URL||"");
if(base.protocol!=="https:"&&!["localhost","127.0.0.1"].includes(base.hostname))throw new Error("HTTPS required");
const orgCode=process.env.ASSETFLOW_ORG_CODE;
const employee=process.env.ASSETFLOW_ADMIN_EMPLOYEE_ID;
const password=process.env.ASSETFLOW_ADMIN_PASSWORD;
if(!orgCode||!employee||!password)throw new Error("Organization code, admin employee ID and password required");
let cookie="";
async function api(path,body){
 const response=await fetch(new URL("/api"+path,base),{
  method:"POST",redirect:"error",
  headers:{"Content-Type":"application/json","X-Requested-With":"AssetFlow",Origin:base.origin,...(cookie?{Cookie:cookie}:{})},
  body:JSON.stringify(body),
 });
 const data=await response.json();
 if(!response.ok)throw new Error(`${path}: ${response.status} ${data.error||"Request failed"}`);
 if(path==="/login"){
  const session=response.headers.getSetCookie().find(v=>v.startsWith("assetflow_session="));
  if(!session)throw new Error("Session cookie missing");cookie=session.split(";",1)[0];
 }
 return data;
}
const account=await api("/login",{org_code:orgCode,employee_id:employee,password});
if(account.role!=="admin"||!account.all_branches)throw new Error("Central administrator required");
try{console.log(JSON.stringify(await api("/demo-data",{confirmation}),null,2));}
finally{await api("/logout",{});}
