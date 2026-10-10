<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { Download, Plus, RefreshCw, Save, Check, X, Archive, FileText, Landmark, CalendarClock, Scale, ShieldCheck } from "lucide-vue-next";
import { api, date, money } from "./api";
import { t } from "./preferences";

const props = defineProps<{ branchId: number; userId: number; finance: boolean; canManage: boolean; canPropose: boolean; canDecide: boolean; contractsRead: boolean; contractsManage: boolean; locale: string }>();
function copy(id: string, en: string) { return props.locale === "id" ? id : en; }
const tab = ref(props.finance ? "depreciation" : "compliance");
const period = ref(new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Jakarta" }).slice(0, 7));
const loading = ref(false), saving = ref(false), error = ref(""), notice = ref("");
const rows = ref<any[]>([]), assets = ref<any[]>([]), compliance = ref<Record<string, any>>({});
const accounts = reactive<Record<string, string>>({ asset_account:"1500", accumulated_depreciation_account:"1590", depreciation_expense_account:"6000", cash_account:"1100", disposal_gain_account:"7990", disposal_loss_account:"6990", revaluation_reserve_account:"3100", impairment_expense_account:"6900" });
const accountFields = computed(() => [["asset_account",copy("Harga perolehan aset","Asset cost")],["accumulated_depreciation_account",copy("Akumulasi penyusutan","Accumulated depreciation")],["depreciation_expense_account",copy("Beban penyusutan","Depreciation expense")],["cash_account",copy("Kas / hasil pelepasan","Cash / proceeds")],["disposal_gain_account",copy("Keuntungan pelepasan","Disposal gain")],["disposal_loss_account",copy("Kerugian pelepasan","Disposal loss")],["revaluation_reserve_account",copy("Cadangan revaluasi","Revaluation reserve")],["impairment_expense_account",copy("Beban penurunan nilai","Impairment expense")] ]);
type FinanceTab = [string, string, any];
const showValuation = ref(false), showContract = ref(false);
const valuationForm = reactive({ asset_id:0, revalued_amount:0, remaining_life_months:0, reason:"" });
const contractForm = reactive({ branch_id:props.branchId, asset_id:null as number|null, name:"", vendor:"", contract_number:"", start_date:"", end_date:"", renewal_notice_days:30, annual_cost:0, notes:"" });
const tabs = computed<FinanceTab[]>(() => {
  const items: FinanceTab[] = [];
  if (props.finance) items.push(["depreciation",t("finance.depreciation"),Scale]);
  if (props.canPropose) items.push(["valuations",t("finance.valuations"),Landmark]);
  if (props.contractsRead) items.push(["contracts",t("finance.contracts"),CalendarClock]);
  if (props.finance) items.push(["journals",t("finance.journals"),FileText]);
  if (props.canManage) items.push(["settings",t("finance.settings"),Landmark]);
  items.push(["compliance",t("finance.compliance"),ShieldCheck]);
  return items;
});
function branchQuery() { return "branch_id=" + props.branchId + "&"; }
async function load() {
  loading.value=true; error.value="";
  try {
    if(tab.value==="depreciation") rows.value=(await api("/finance/depreciation?"+branchQuery()+"period="+period.value)).items||[];
    else if(tab.value==="valuations") {
      const result=await Promise.all([api("/finance/valuations?"+branchQuery()+"size=100"),api("/assets?"+branchQuery()+"size=100")]);
      rows.value=result[0]; assets.value=result[1].items||[];
    } else if(tab.value==="contracts") {
      const result=await Promise.all([api("/contracts?"+branchQuery()+"size=100"),api("/assets?"+branchQuery()+"size=100")]);
      rows.value=result[0]; assets.value=result[1].items||[];
    } else if(tab.value==="journals") rows.value=(await api("/finance/journals?"+branchQuery()+"period="+period.value)).lines||[];
    else if(tab.value==="settings") { const result=await api<any[]>("/finance/settings"); if(result[0]) Object.assign(accounts,result[0]); }
    else if(tab.value==="compliance") compliance.value=await api("/reports/compliance?branch_id="+props.branchId);
  } catch(cause) { error.value=(cause as Error).message; }
  finally { loading.value=false; }
}
watch(() => [tab.value,period.value,props.branchId,props.locale],() => void load(),{immediate:true});
async function saveValuation() {
  saving.value=true; error.value="";
  try { await api("/finance/valuations",{...valuationForm}); showValuation.value=false; notice.value=t("finance.pending"); await load(); }
  catch(cause) { error.value=(cause as Error).message; } finally { saving.value=false; }
}
async function decide(row:any,approve:boolean) {
  const note=approve ? "" : window.prompt(props.locale==="id" ? "Alasan penolakan" : "Reason for rejection","");
  if(!approve&&!note) return;
  try { await api("/finance/valuations/"+row.id+"/decision",{approve,note:note||""}); await load(); }
  catch(cause) { error.value=(cause as Error).message; }
}
async function saveContract() {
  saving.value=true; error.value="";
  try {
    await api("/contracts",{...contractForm}); showContract.value=false;
    Object.assign(contractForm,{branch_id:props.branchId,asset_id:null,name:"",vendor:"",contract_number:"",start_date:"",end_date:"",renewal_notice_days:30,annual_cost:0,notes:""});
    await load();
  } catch(cause) { error.value=(cause as Error).message; } finally { saving.value=false; }
}
async function archiveContract(row:any) {
  if(!window.confirm((props.locale==="id"?"Arsipkan kontrak ":"Archive contract ")+row.name+"?")) return;
  try { await api("/contracts/"+row.id,undefined,"DELETE"); await load(); } catch(cause) { error.value=(cause as Error).message; }
}
async function saveAccounts() {
  saving.value=true; error.value="";
  try { await api("/finance/settings",{...accounts}); notice.value=t("common.save"); } catch(cause) { error.value=(cause as Error).message; } finally { saving.value=false; }
}
async function exportJournal() {
  saving.value=true; error.value="";
  try {
    const response=await fetch("/api/exports/journal",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json","X-Requested-With":"AssetFlow"},body:JSON.stringify({period:period.value,branch_id:props.branchId})});
    if(!response.ok) throw new Error((await response.json()).error||"Export failed");
    const url=URL.createObjectURL(await response.blob()), link=document.createElement("a"); link.href=url; link.download="asset-journal-"+period.value+".csv"; link.click(); URL.revokeObjectURL(url); notice.value=t("finance.export");
  } catch(cause) { error.value=(cause as Error).message; } finally { saving.value=false; }
}
</script>

<template>
  <section class="finance-workspace">
    <div class="finance-toolbar">
      <div class="finance-tabs" role="tablist"><button v-for="item in tabs" :key="item[0]" role="tab" :aria-selected="tab===item[0]" :class="{active:tab===item[0]}" @click="tab=item[0]"><component :is="item[2]" :size="16"/>{{item[1]}}</button></div>
      <div class="finance-actions">
        <label v-if="['depreciation','journals'].includes(tab)" class="period-field">{{t("common.period")}}<input v-model="period" type="month"/></label>
        <button v-if="tab==='valuations'&&canPropose" class="primary" @click="showValuation=true"><Plus :size="16"/>{{t("finance.propose")}}</button>
        <button v-if="tab==='contracts'&&contractsManage" class="primary" @click="showContract=true"><Plus :size="16"/>{{t("finance.addContract")}}</button>
        <button v-if="tab==='journals'" class="secondary" :disabled="saving" @click="exportJournal"><Download :size="16"/>{{t("finance.export")}}</button>
        <button v-if="tab==='settings'&&canManage" class="primary" :disabled="saving" @click="saveAccounts"><Save :size="16"/>{{t("finance.saveAccounts")}}</button>
        <button class="icon" :aria-label="t('common.refresh')" @click="load"><RefreshCw :size="17"/></button>
      </div>
    </div>
    <div v-if="error" class="alert" role="alert">{{error}}</div>
    <div v-if="notice" class="finance-notice" role="status">{{notice}}<button class="icon" @click="notice=''"><X :size="15"/></button></div>
    <div v-if="loading" class="loading" aria-live="polite">{{t("common.loading")}}</div>
    <template v-if="tab==='depreciation'">
      <div class="finance-summary"><div><small>{{t("common.period")}}</small><strong>{{period}}</strong></div><p>{{t("finance.disclaimer")}}</p></div>
      <div class="table-scroll"><table><thead><tr><th>{{t("asset.tag")}}</th><th>{{t("common.branch")}}</th><th>{{t("asset.method")}}</th><th>{{props.locale==='id'?'Nilai buku awal':'Opening book value'}}</th><th>{{props.locale==='id'?'Revaluasi':'Revaluation'}}</th><th>{{props.locale==='id'?'Beban periode':'Period charge'}}</th><th>{{props.locale==='id'?'Nilai buku akhir':'Closing book value'}}</th></tr></thead><tbody>
        <tr v-for="row in rows" :key="row.asset_id"><td><b>{{row.name}}</b><small>{{row.tag}}</small></td><td>{{row.branch_code}}</td><td>{{row.depreciation_method==='straight_line'?t('asset.straight'):row.depreciation_method==='declining_balance'?t('asset.declining'):t('asset.nonDepreciable')}}</td><td>{{money(row.opening_book_value)}}</td><td>{{money(row.revaluation_change)}}</td><td>{{money(row.depreciation_charge)}}</td><td>{{money(row.closing_book_value)}}</td></tr>
        <tr v-if="!loading&&!rows.length"><td colspan="7" class="empty">{{t("finance.empty")}}</td></tr>
      </tbody></table></div>
    </template>
    <template v-else-if="tab==='valuations'">
      <div class="table-scroll"><table><thead><tr><th>{{copy("Aset","Asset")}}</th><th>{{t("common.branch")}}</th><th>{{copy("Berlaku","Effective")}}</th><th>{{copy("Nilai lama → baru","Old → new value")}}</th><th>{{t("common.reason")}}</th><th>{{t("common.status")}}</th><th>{{t("common.actions")}}</th></tr></thead><tbody>
        <tr v-for="row in rows" :key="row.id"><td><b>{{row.name}}</b><small>{{row.tag}}</small></td><td>{{row.branch_code}}</td><td>{{date(row.effective_date)}}</td><td>{{row.carrying_value_before==null?'—':money(row.carrying_value_before)+' → '+money(row.revalued_amount)}}</td><td>{{row.reason}}</td><td><span class="badge" :class="row.status">{{t('status.'+row.status)}}</span></td><td><div class="row-actions" v-if="row.status==='pending'&&canDecide&&row.requested_by!==userId"><button @click="decide(row,true)"><Check :size="14"/>{{t("finance.approve")}}</button><button @click="decide(row,false)"><X :size="14"/>{{t("finance.reject")}}</button></div><small v-else-if="row.status==='pending'">{{t("finance.pending")}}</small></td></tr>
        <tr v-if="!loading&&!rows.length"><td colspan="7" class="empty">{{t("finance.empty")}}</td></tr>
      </tbody></table></div>
      <div v-if="showValuation" class="finance-form-wrap"><form class="finance-form" @submit.prevent="saveValuation"><h3>{{t("finance.propose")}}</h3><label>{{copy("Aset","Asset")}}<select v-model.number="valuationForm.asset_id" required><option :value="0" disabled>{{copy("Pilih aset","Select asset")}}</option><option v-for="a in assets.filter((item)=>item.status!=='disposed')" :key="a.id" :value="a.id">{{a.tag}} · {{a.name}}</option></select></label><label>{{copy("Nilai baru (Rp)","New value (IDR)")}}<input v-model.number="valuationForm.revalued_amount" type="number" min="0" step="1" required/></label><label>{{copy("Sisa masa manfaat (bulan, 0 untuk otomatis)","Remaining useful life (months, 0 for automatic)")}}<input v-model.number="valuationForm.remaining_life_months" type="number" min="0" max="1200"/></label><label>{{t("common.reason")}}<textarea v-model="valuationForm.reason" minlength="3" maxlength="1000" required/></label><div class="finance-form-actions"><button type="button" class="secondary" @click="showValuation=false">{{t("common.cancel")}}</button><button class="primary" :disabled="saving">{{t("finance.propose")}}</button></div></form></div>
    </template>
    <template v-else-if="tab==='contracts'">
      <div class="table-scroll"><table><thead><tr><th>{{t("common.name")}}</th><th>{{copy("Pemasok / nomor","Vendor / number")}}</th><th>{{copy("Aset","Asset")}}</th><th>{{copy("Masa berlaku","Term")}}</th><th>{{copy("Biaya tahunan","Annual cost")}}</th><th>{{copy("Perpanjangan","Renewal")}}</th><th>{{t("common.actions")}}</th></tr></thead><tbody>
        <tr v-for="row in rows" :key="row.id"><td><b>{{row.name}}</b><small>{{row.branch_code}}</small></td><td>{{row.vendor}}<small>{{row.contract_number||'—'}}</small></td><td>{{row.tag||(props.locale==='id'?'Seluruh cabang':'Branch-wide')}}</td><td>{{date(row.start_date)}} – {{date(row.end_date)}}</td><td>{{row.annual_cost==null?'—':money(row.annual_cost)}}</td><td><span class="badge" :class="row.renewal_due?'maintenance':'completed'">{{row.renewal_due?(props.locale==='id'?'Segera':'Due soon'):(props.locale==='id'?'Terpantau':'On track')}}</span></td><td><button v-if="contractsManage" class="text-btn" @click="archiveContract(row)"><Archive :size="14"/>{{t("common.archive")}}</button></td></tr>
        <tr v-if="!loading&&!rows.length"><td colspan="7" class="empty">{{t("finance.empty")}}</td></tr>
      </tbody></table></div>
      <div v-if="showContract" class="finance-form-wrap"><form class="finance-form" @submit.prevent="saveContract"><h3>{{t("finance.addContract")}}</h3><label>{{t("common.name")}}<input v-model="contractForm.name" required minlength="2" maxlength="200"/></label><label>{{copy("Pemasok","Vendor")}}<input v-model="contractForm.vendor" required minlength="2" maxlength="200"/></label><label>{{copy("Aset","Asset")}}<select v-model="contractForm.asset_id"><option :value="null">{{copy("Kontrak tingkat cabang","Branch-wide contract")}}</option><option v-for="a in assets" :key="a.id" :value="a.id">{{a.tag}} · {{a.name}}</option></select></label><label>{{copy("Nomor kontrak","Contract number")}}<input v-model="contractForm.contract_number" maxlength="100"/></label><div class="form-grid"><label>{{copy("Mulai","Start")}}<input v-model="contractForm.start_date" type="date" required/></label><label>{{copy("Berakhir","End")}}<input v-model="contractForm.end_date" type="date" :min="contractForm.start_date" required/></label></div><label>{{copy("Pemberitahuan perpanjangan (hari)","Renewal notice (days)")}}<input v-model.number="contractForm.renewal_notice_days" type="number" min="0" max="3650"/></label><label v-if="finance">{{copy("Biaya tahunan (Rp)","Annual cost (IDR)")}}<input v-model.number="contractForm.annual_cost" type="number" min="0" step="1"/></label><label>{{copy("Catatan","Notes")}}<textarea v-model="contractForm.notes" maxlength="2000"/></label><div class="finance-form-actions"><button type="button" class="secondary" @click="showContract=false">{{t("common.cancel")}}</button><button class="primary" :disabled="saving">{{t("common.save")}}</button></div></form></div>
    </template>
    <template v-else-if="tab==='journals'"><div class="finance-summary"><p>{{t("finance.disclaimer")}}</p></div><div class="table-scroll"><table><thead><tr><th>{{copy("Tanggal","Date")}}</th><th>{{t("common.branch")}}</th><th>{{copy("Referensi","Reference")}}</th><th>{{copy("Akun","Account")}}</th><th>{{copy("Debit","Debit")}}</th><th>{{copy("Kredit","Credit")}}</th><th>{{copy("Uraian","Memo")}}</th></tr></thead><tbody><tr v-for="(row,index) in rows" :key="row.reference+'-'+index"><td>{{date(row.journal_date)}}</td><td>{{row.branch_code}}</td><td>{{row.reference}}</td><td>{{row.account_code}}</td><td>{{row.debit?money(row.debit):'—'}}</td><td>{{row.credit?money(row.credit):'—'}}</td><td>{{row.memo}}</td></tr><tr v-if="!loading&&!rows.length"><td colspan="7" class="empty">{{t("finance.empty")}}</td></tr></tbody></table></div></template>
    <template v-else-if="tab==='settings'"><p class="hint">{{t("finance.disclaimer")}}</p><form class="account-grid" @submit.prevent="saveAccounts"><label v-for="field in accountFields" :key="field[0]">{{field[1]}}<input v-model="accounts[field[0]]" maxlength="40" pattern="[A-Za-z0-9._-]{1,40}" required/></label></form></template>
    <template v-else-if="tab==='compliance'"><div class="compliance-grid"><article v-for="item in [['assets_active',props.locale==='id'?'Aset aktif':'Active assets'],['assets_archived',props.locale==='id'?'Aset diarsipkan':'Archived assets'],['assets_missing_tag',props.locale==='id'?'Tanpa tag':'Missing tag'],['warranties_expired',props.locale==='id'?'Garansi berakhir':'Expired warranties'],['maintenance_overdue',props.locale==='id'?'Pemeliharaan terlambat':'Overdue maintenance'],['contracts_expired',props.locale==='id'?'Kontrak berakhir':'Expired contracts']]" :key="item[0]"><small>{{item[1]}}</small><strong>{{compliance[item[0]]??0}}</strong></article></div><p class="hint">{{props.locale==='id'?'Ringkasan per':'Snapshot as of'}} {{compliance.as_of||'—'}}</p></template>
  </section>
</template>
