const { navigate } = require("./navigation.cjs");
const path = require("node:path");

function fixtures() {
 const stamp={created_at:"2026-10-11T00:00:00Z",updated_at:"2026-10-11T00:00:00Z",created_by:1,updated_by:1,version:1,deleted_at:null};
 const groups=[{...stamp,id:1,code:"COMPANY",name:"Example company",company_prefix:"MJ",description:"",detail_count:1}];
 const details=[{...stamp,id:1,general_code_id:1,code:"BRANCH",name:"Branches",prefix:"CBG",entity_type:"branch",sequence_width:6,next_number:1,issued_count:0}];
 return req=>{
  const url=new URL(req.url()), endpoint=url.pathname.replace("/api","");
  if(endpoint==="/demo-data")return {assets:26,branch_code:"DEMO",already_seeded:true};
  if(endpoint==="/general-codes/issued")return [];
  const match=endpoint.match(/^\/(general-codes|general-code-details)(?:\/(\d+))?(\/restore)?$/);
  if(!match)return undefined;
  const collection=match[1]==="general-codes"?groups:details;
  const id=Number(match[2]);
  if(req.method()==="GET"){
   const archived=url.searchParams.get("archived")==="true";
   return collection.filter(r=>!!r.deleted_at===archived).map(r=>{
    const parent=groups.find(g=>g.id===r.general_code_id);
    return {...r,general_name:parent?.name,general_code:parent?.code,company_prefix:r.company_prefix||parent?.company_prefix,preview:parent?`${parent.company_prefix}-${r.prefix}-${String(r.next_number).padStart(r.sequence_width,"0")}`:""};
   });
  }
  const body=JSON.parse(req.postData()||"{}");
  const row=collection.find(r=>r.id===id);
  if(req.method()==="DELETE"){row.deleted_at=stamp.created_at;return {ok:true};}
  if(match[3]){row.deleted_at=null;return {ok:true};}
  if(req.method()==="PUT"){Object.assign(row,body,{version:row.version+1});return {id};}
  const next=collection.length+1;
  collection.push({...stamp,...body,id:next,code:body.code.toUpperCase(),next_number:1,issued_count:0,detail_count:0});
  return {id:next};
 };
}

async function exercise(page,check,getLastPost,directory) {
 await navigate(page,"General Code");
 await page.locator(".general-setup .loading").waitFor({state:"hidden"});
 await page.locator(".general-setup .primary").click();
 let dialog=page.getByRole("dialog");
 await page.getByLabel("Code",{exact:true}).fill("NEW_GROUP");
 await page.getByLabel("Name",{exact:true}).fill("New company rules");
 await page.getByLabel("Company prefix",{exact:true}).fill("TEST");
 check(await page.locator(".workspace").getAttribute("inert")!==null,"setup modal isolates background");
 await dialog.getByRole("button",{name:"Save",exact:true}).click();
 await dialog.waitFor({state:"hidden"});
 check(getLastPost().path==="/api/general-codes"&&getLastPost().body.company_prefix==="TEST","general code create payload");
 await page.getByRole("button",{name:"Edit NEW_GROUP",exact:true}).click();
 await page.getByLabel("Name",{exact:true}).fill("Renamed company rules");
 check(await page.getByLabel("Code",{exact:true}).isDisabled(),"general code identity immutable");
 await page.getByRole("dialog").getByRole("button",{name:"Save",exact:true}).click();
 await page.getByRole("dialog").waitFor({state:"hidden"});
 check(getLastPost().method==="PUT"&&getLastPost().body.version===1,"general code edit version guard");
 page.once("dialog",d=>d.accept());
 await page.getByRole("button",{name:"Archive NEW_GROUP",exact:true}).click();
 await page.locator(".general-setup .loading").waitFor({state:"hidden"});
 await page.getByLabel("Archived",{exact:true}).check();
 await page.getByRole("button",{name:"Restore NEW_GROUP",exact:true}).waitFor();
 page.once("dialog",d=>d.accept());
 await page.getByRole("button",{name:"Restore NEW_GROUP",exact:true}).click();
 await page.getByLabel("Archived",{exact:true}).uncheck();
 await navigate(page,"General Code Detail");
 await page.locator(".general-setup .loading").waitFor({state:"hidden"});
 await page.locator(".general-setup .primary").click();
 await page.getByLabel("Code",{exact:true}).fill("HEAD_OFFICE");
 await page.getByLabel("Name",{exact:true}).fill("Head office identifiers");
 await page.getByLabel(/Entity type/).selectOption("hq");
 await page.getByLabel("Prefix",{exact:true}).fill("PST");
 check((await page.locator(".code-preview").textContent()).includes("MJ-PST-000001"),"code format preview");
 await page.getByRole("dialog").getByRole("button",{name:"Save",exact:true}).click();
 await page.getByRole("dialog").waitFor({state:"hidden"});
 check(getLastPost().body.entity_type==="hq"&&getLastPost().body.general_code_id===1,"HQ rule belongs to company group");
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:900});
  await page.screenshot({path:path.join(directory,`general-setup-${width}.png`),fullPage:true});
  check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),"general setup responsive "+width);
  await page.getByRole("button",{name:"Edit HEAD_OFFICE",exact:true}).click();
  check(await page.getByRole("dialog").evaluate(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth+1&&r.bottom<=innerHeight+1;}),"general detail modal fits "+width);
  await page.keyboard.press("Escape");
  await page.getByRole("dialog").waitFor({state:"hidden"});
 }
 await page.setViewportSize({width:1440,height:1040});
 for(const [nav,resource] of [["Branches","branches"],["Locations","locations"],["Categories","categories"],["Team & access","users"]]){
  await navigate(page,nav);
  await page.locator(".main table tbody tr").first().getByRole("button",{name:"Edit",exact:true}).click();
  await page.getByLabel("Name",{exact:true}).fill("Renamed master record");
  await page.getByRole("button",{name:"Save & confirm",exact:true}).click();
  await page.getByRole("dialog").waitFor({state:"hidden"});
  check(getLastPost().method==="PUT"&&getLastPost().path.startsWith("/api/"+resource+"/")&&getLastPost().body.version===1,"master "+resource+" versioned edit");
 }
}
module.exports={fixtures,exercise};
