<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from "vue";
import { Plus, Pencil, Archive, ArchiveRestore, RefreshCw, X, Database, Save } from "lucide-vue-next";
import { api, timestamp } from "./api";
import { locale } from "./preferences";
const props = defineProps<{view:string;canManage:boolean}>();
const emit = defineEmits<{seeded:[];dialog:[open:boolean]}>();
const copy=(id:string,en:string)=>locale.value==="id"?id:en;
const detail=computed(()=>props.view==="general-code-details");
const rows=ref<any[]>([]), parents=ref<any[]>([]), issued=ref<any[]>([]);
const archived=ref(false), busy=ref(false), saving=ref(false), error=ref(""), message=ref(""), modal=ref(false);
const form=ref<Record<string,any>>({});
const endpoint=computed(()=>detail.value?"/general-code-details":"/general-codes");
const fixedFormat=computed(()=>detail.value&&form.value.next_number>1);
const preview=computed(()=>{
  const prefix=detail.value?parents.value.find(g=>g.id===form.value.general_code_id)?.company_prefix:form.value.company_prefix;
  return detail.value&&prefix ? `${prefix}-${form.value.prefix || "..."}-${String(form.value.next_number || 1).padStart(form.value.sequence_width || 6,"0")}` : prefix || "";
});
async function load(){
 busy.value=true;error.value="";
 try{
  const [list,groups,allocations]=await Promise.all([api(endpoint.value+"?archived="+archived.value),api("/general-codes"),api("/general-codes/issued")]);
  rows.value=list;parents.value=groups;issued.value=allocations;
 }catch(e){error.value=(e as Error).message;}
 finally{busy.value=false;}
}
let focus:HTMLElement|null=null;let overflow="";
watch(modal,async open=>{
 emit("dialog",open);
 if(open){focus=document.activeElement as HTMLElement;overflow=document.body.style.overflow;document.body.style.overflow="hidden";await nextTick();document.querySelector<HTMLElement>(".setup-dialog input:not([disabled]),.setup-dialog select:not([disabled]),.setup-dialog button")?.focus();}
 else{document.body.style.overflow=overflow;await nextTick();focus?.focus();}
});
function open(row?:any){
 error.value="";
 form.value=row?{...row}:detail.value?{general_code_id:parents.value[0]?.id,code:"",name:"",prefix:"",entity_type:"branch",sequence_width:6,version:1}:{code:"",name:"",company_prefix:"",description:"",version:1};
 modal.value=true;
}
function keydown(event:KeyboardEvent){
 if(event.key==="Escape"&&!saving.value){modal.value=false;return;}
 if(event.key!=="Tab")return;
 const buttons=Array.from((event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>("input:not([disabled]),textarea:not([disabled]),select:not([disabled]),button:not([disabled])")).filter(e=>e.offsetParent!==null);
 if(event.shiftKey&&document.activeElement===buttons[0]){event.preventDefault();buttons.at(-1)?.focus();}
 else if(!event.shiftKey&&document.activeElement===buttons.at(-1)){event.preventDefault();buttons[0]?.focus();}
}
async function save(){
 if(saving.value)return;saving.value=true;error.value="";
 try{
  const f=form.value;
  const body=detail.value?{general_code_id:f.general_code_id,code:f.code,name:f.name,prefix:f.prefix,entity_type:f.entity_type,sequence_width:f.sequence_width,version:f.version}:{code:f.code,name:f.name,company_prefix:f.company_prefix,description:f.description,version:f.version};
  await api(endpoint.value+(f.id?"/"+f.id:""),body,f.id?"PUT":"POST");
  modal.value=false;message.value=copy("Data tersimpan","Data saved");await load();
 }catch(e){error.value=(e as Error).message;}finally{saving.value=false;}
}
async function archive(row:any){
 if(!window.confirm(copy(`${archived.value?"Pulihkan":"Arsipkan"} ${row.name}?`,`${archived.value?"Restore":"Archive"} ${row.name}?`)))return;
 saving.value=true;error.value="";
 try{await api(endpoint.value+"/"+row.id+(archived.value?"/restore":""),archived.value?{}:undefined,archived.value?"POST":"DELETE");await load();}
 catch(e){error.value=(e as Error).message;}finally{saving.value=false;}
}
async function seed(){
 if(!window.confirm(copy("Tambahkan sampel fiktif pada cabang DEMO tanpa menghapus data lama? Data DEMO akan masuk ke laporan perusahaan.","Add fictional samples in the DEMO branch without deleting existing data? DEMO will be included in company reports.")))return;
 saving.value=true;error.value="";
 try{const result=await api("/demo-data",{confirmation:"SEED_DEMO_KEEP_EXISTING"});message.value=copy(`DEMO: ${result.assets} aset. ${result.already_seeded?"Data sudah tersedia.":"Data ditambahkan."}`,`DEMO: ${result.assets} assets. ${result.already_seeded?"Data already exists.":"Data added."}`);emit("seeded");await load();}
 catch(e){error.value=(e as Error).message;}finally{saving.value=false;}
}
watch(()=>props.view,load,{immediate:true});
onUnmounted(()=>{if(modal.value){document.body.style.overflow=overflow;emit("dialog",false);}});
</script>

<template>
 <section class="general-setup">
  <div class="setup-toolbar">
   <label class="check-label"><input v-model="archived" type="checkbox" :disabled="busy||saving" @change="load" />{{copy("Arsip","Archived")}}</label>
   <div class="setup-actions">
    <button class="icon-btn" :title="copy('Muat ulang','Refresh')" :aria-label="copy('Muat ulang','Refresh')" :disabled="busy||saving" @click="load"><RefreshCw :size="17" /></button>
    <button v-if="canManage&&!detail" class="secondary" :disabled="busy||saving" @click="seed"><Database :size="17" />{{copy("Data sampel DEMO","DEMO sample data")}}</button>
    <button v-if="canManage&&!archived" class="primary" :disabled="busy||saving||(detail&&!parents.length)" @click="open()"><Plus :size="17" />{{detail?"General Code Detail":"General Code"}}</button>
   </div>
  </div>
  <p v-if="message" role="status">{{message}}</p>
  <div v-if="error&&!modal" class="alert" role="alert">{{error}}</div>
  <div v-if="busy" class="loading" role="status">{{copy("Memuat data...","Loading...")}}</div>
  <div v-else class="table-wrap">
   <table><thead><tr><th>{{copy("Kode / nama","Code / name")}}</th><th>{{detail?"General Code":copy("Prefix perusahaan","Company prefix")}}</th><th v-if="detail">{{copy("Jenis","Type")}}</th><th>{{detail?copy("Nomor berikutnya","Next code"):copy("Detail aktif","Active details")}}</th><th>{{copy("Pembaruan","Updated")}}</th><th>{{copy("Tindakan","Actions")}}</th></tr></thead>
   <tbody><tr v-for="row in rows" :key="row.id">
    <td><strong>{{row.code}}</strong><small>{{row.name}}</small></td><td>{{detail?row.general_name:row.company_prefix}}</td><td v-if="detail">{{row.entity_type}}</td>
    <td>{{detail?(String(row.next_number).length>row.sequence_width?copy("Nomor habis","Exhausted"):row.preview):row.detail_count}}<small v-if="detail">{{row.issued_count}} {{copy("kode terbit","codes issued")}}</small></td>
    <td>{{timestamp(row.updated_at)}}<small>{{copy("Oleh","By")}} #{{row.updated_by || row.created_by || "-"}}</small></td>
    <td><div class="setup-actions">
     <button v-if="canManage&&!archived" class="icon-btn" :title="copy('Edit','Edit')" :aria-label="copy('Edit','Edit')+' '+row.code" :disabled="saving" @click="open(row)"><Pencil :size="17" /></button>
     <button v-if="canManage" class="icon-btn" :title="archived?copy('Pulihkan','Restore'):copy('Arsipkan','Archive')" :aria-label="(archived?copy('Pulihkan','Restore'):copy('Arsipkan','Archive'))+' '+row.code" :disabled="saving" @click="archive(row)"><ArchiveRestore v-if="archived" :size="17" /><Archive v-else :size="17" /></button>
    </div></td>
   </tr></tbody></table><div v-if="!rows.length" class="empty">{{copy("Belum ada data.","No records yet.")}}</div>
  </div>
  <template v-if="detail&&issued.length"><h2 class="issued-title">{{copy("Kode terbit","Issued codes")}}</h2><div class="table-wrap"><table><thead><tr><th>{{copy("Kode","Code")}}</th><th>{{copy("Aturan","Rule")}}</th><th>{{copy("Objek","Entity")}}</th><th>{{copy("Waktu","Time")}}</th></tr></thead><tbody><tr v-for="row in issued" :key="row.id"><td>{{row.code}}</td><td>{{row.detail_name}}</td><td>{{row.entity_type}} #{{row.entity_id}}</td><td>{{timestamp(row.created_at)}}</td></tr></tbody></table></div></template>
 </section>
 <Teleport to="body"><div v-if="modal" class="modal-backdrop" @click.self="!saving&&(modal=false)">
  <div class="modal setup-dialog" role="dialog" aria-modal="true" aria-labelledby="setup-title" @keydown="keydown">
   <div class="modal-heading"><h2 id="setup-title">{{form.id?copy("Edit","Edit"):copy("Tambah","Add")}} {{detail?"General Code Detail":"General Code"}}</h2><button class="icon-btn" :disabled="saving" :title="copy('Tutup','Close')" :aria-label="copy('Tutup','Close')" @click="modal=false"><X :size="20" /></button></div>
   <form @submit.prevent="save">
    <label v-if="detail">General Code<select v-model.number="form.general_code_id" required :disabled="!!form.id"><option v-for="g in parents" :key="g.id" :value="g.id">{{g.code}} · {{g.name}}</option></select></label>
    <label>{{copy("Kode","Code")}}<input v-model="form.code" pattern="[A-Za-z][A-Za-z0-9_]{1,39}" maxlength="40" required :disabled="!!form.id" /></label>
    <label>{{copy("Nama","Name")}}<input v-model="form.name" minlength="2" maxlength="150" required /></label>
    <template v-if="detail">
     <label>{{copy("Jenis objek","Entity type")}}<select v-model="form.entity_type" :disabled="fixedFormat"><option value="hq">{{copy("Pusat","Head office")}}</option><option value="branch">{{copy("Cabang","Branch")}}</option><option value="reference">{{copy("Referensi","Reference")}}</option></select></label>
     <label>Prefix<input v-model="form.prefix" pattern="[A-Za-z0-9]{1,8}" maxlength="8" required :disabled="fixedFormat" /></label>
     <label>{{copy("Jumlah digit","Sequence digits")}}<input v-model.number="form.sequence_width" type="number" min="3" max="8" required :disabled="fixedFormat" /></label>
    </template>
    <template v-else><label>{{copy("Prefix perusahaan","Company prefix")}}<input v-model="form.company_prefix" pattern="[A-Za-z0-9]{1,8}" maxlength="8" required /></label><label>{{copy("Deskripsi","Description")}}<textarea v-model="form.description" maxlength="1000"></textarea></label></template>
    <output v-if="detail" class="code-preview">{{preview}}</output>
    <div v-if="error" class="alert" role="alert">{{error}}</div>
    <div class="modal-footer"><button type="button" class="secondary" :disabled="saving" @click="modal=false">{{copy("Batal","Cancel")}}</button><button type="submit" class="primary" :disabled="saving"><Save :size="17" />{{copy("Simpan","Save")}}</button></div>
   </form>
  </div>
 </div></Teleport>
</template>

<style scoped>
.general-setup{min-width:0;max-width:100%}
.table-wrap{width:100%;max-width:100%;overflow-x:auto;border:1px solid var(--line);border-radius:4px;background:var(--surface)}
.table-wrap table{min-width:740px}
.table-wrap th{white-space:nowrap}
.table-wrap td{max-width:220px;overflow-wrap:anywhere}
.icon-btn{width:34px;height:34px;flex:none;padding:0;border:1px solid var(--line);border-radius:4px;background:var(--surface);color:var(--ink)}
.setup-toolbar{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:18px;flex-wrap:wrap}
.setup-actions{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.setup-toolbar .check-label{margin:0}
.issued-title{font-size:18px;margin:26px 0 14px}
.code-preview{display:block;font-family:monospace;padding:12px;background:var(--surface-muted);color:var(--ink);border:1px solid var(--line);overflow-wrap:anywhere}
@media(max-width:600px){.setup-toolbar{align-items:flex-start}.setup-actions{max-width:100%}.setup-toolbar .setup-actions{width:100%}}
</style>
