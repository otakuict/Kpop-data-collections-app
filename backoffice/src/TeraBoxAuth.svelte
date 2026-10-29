<script>
 import { onMount } from 'svelte';
 import { Check, X } from '@lucide/svelte';
 import { parseTeraBoxCurl } from './teraboxCurl.js';
 let { api, onclose } = $props();
 let accounts=$state([]), accountID=$state(1), working=$state(false);
 let label=$state(''), root=$state('/'), source=$state(''), host=$state('www.terabox.com');
 let cookie=$state(''), bdstoken=$state(''), jsToken=$state('');
 let error=$state(''), notice=$state('');
 let curlOpen=$state(false), curlInput=$state(''), curlError=$state('');
 function modal(node){node.showModal();return {destroy(){node.close();}};}
 function clearSecrets(){cookie='';bdstoken='';jsToken='';curlInput='';curlError='';}
 function changeAccount(){
  const account=accounts.find(a=>a.id===accountID);
  label=account?.label||`Account ${accountID}`;
  root=account?.root||'/';source=account?.source||'';
  host=account?.host||'www.terabox.com';
  clearSecrets();error='';notice='';
 }
 function closeAuth(){if(working)return;curlOpen=false;clearSecrets();onclose();}
 function closeCurlImport(){curlOpen=false;curlInput='';curlError='';}
 function applyCurl(event){
  event.preventDefault();
  try{
   const parsed=parseTeraBoxCurl(curlInput);
   cookie=parsed.cookie;jsToken=parsed.jsToken;bdstoken=parsed.bdstoken;host=parsed.host;
   closeCurlImport();error='';notice='Session fields filled from cURL. Save the account session to connect.';
  }catch(e){curlError=e.message;}
 }
 async function loadAccounts(){const result=await api('/admin/terabox/accounts');accounts=result.accounts;}
 onMount(()=>{working=true;loadAccounts().then(changeAccount).catch(e=>error=e.message).finally(()=>working=false);return clearSecrets;});
 async function saveAccount(event){
  event.preventDefault();working=true;error='';notice='';
  try{
   await api(`/admin/terabox/accounts/${accountID}`,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({label,root,source,host,cookie,bdstoken,jsToken})});
   clearSecrets();await loadAccounts();changeAccount();notice='Session saved securely. This account is ready to scan.';
  }catch(e){error=e.message;}finally{working=false;}
 }
</script>

<dialog class="tera-auth-dialog" use:modal oncancel={event=>{event.preventDefault();closeAuth();}} aria-labelledby="tera-auth-title">
 <div class="editor-heading"><div><span class="eyebrow">PRIVATE DRIVE CONNECTION</span><h2 id="tera-auth-title">TeraBox auth</h2></div><button class="icon-btn" aria-label="Close TeraBox auth" disabled={working} onclick={closeAuth}><X size={20}/></button></div>
 <p class="review-note">Connect each account with its logged-in browser session. Credentials are encrypted on the backend and never shown again.</p>
 <label>TeraBox account<select aria-label="TeraBox account" bind:value={accountID} onchange={changeAccount} disabled={working}><option value={1}>{accounts.find(a=>a.id===1)?.label||'Account 1 — not connected'}</option><option value={2}>{accounts.find(a=>a.id===2)?.label||'Account 2 — not connected'}</option></select></label>
 <details><summary>How to connect an account</summary><p>Log in to TeraBox in your browser. Open Developer Tools → Network, open a folder, and select a successful <code>/api/list</code> request. Match its hostname below, then copy the complete Cookie request header and jsToken from that request. Leave bdstoken blank if absent. Do not paste these values into chat.</p></details>
 {#if error}<p class="alert error" role="alert">{error}</p>{/if}
 {#if notice}<p class="alert success" role="status">{notice}</p>{/if}
 <form onsubmit={saveAccount} class="tera-account-form">
  <div class="form-row"><label>Account label<input bind:value={label} placeholder={`Account ${accountID}`} maxlength="80" required disabled={working}/></label><label>Source / seller<input bind:value={source} maxlength="300" placeholder="Source saved with imported sets" disabled={working}/></label></div>
  <label>Root folder path<input bind:value={root} placeholder="/" maxlength="1500" required disabled={working}/><small>Scan and Get names read only entries directly inside this folder.</small></label>
  <label>API host<select bind:value={host} disabled={working}><option value="www.terabox.com">www.terabox.com</option><option value="dm.terabox.com">dm.terabox.com</option><option value="dm.terabox.app">dm.terabox.app</option></select><small>Use the hostname of the successful /api/list request.</small></label>
  <button type="button" class="text-btn tera-curl-trigger" disabled={working} onclick={()=>{curlInput='';curlError='';curlOpen=true;}}>Paste cURL to fill Cookie & jsToken</button>
  <label>Session Cookie<input type="password" autocomplete="off" bind:value={cookie} maxlength="16000" placeholder="Contains ndus=…" required disabled={working}/></label>
  <div class="form-row"><label>bdstoken<input type="password" autocomplete="off" bind:value={bdstoken} maxlength="2000" disabled={working}/></label><label>jsToken<input type="password" autocomplete="off" bind:value={jsToken} maxlength="4000" disabled={working}/></label></div>
  <button class="primary secondary" disabled={working}><Check size={16}/> Save account session</button>
 </form>
</dialog>

{#if curlOpen}
 <dialog class="tera-curl-dialog" use:modal oncancel={event=>{event.preventDefault();closeCurlImport();}} aria-labelledby="tera-curl-title">
  <div class="editor-heading"><h2 id="tera-curl-title">Import TeraBox cURL</h2><button type="button" class="icon-btn" aria-label="Close cURL import" onclick={closeCurlImport}><X size={20}/></button></div>
  <p class="review-note">Paste the Bash cURL command for a successful TeraBox <code>/api/list</code> request. Cookie, jsToken and API host will fill the account form. The command stays in this browser and is cleared when this window closes.</p>
  <form onsubmit={applyCurl}>
   <label>cURL command<textarea bind:value={curlInput} oninput={()=>curlError=''} rows="10" spellcheck="false" autocomplete="off" required placeholder="curl --url 'https://dm.terabox.com/api/list?jsToken=…' -b 'ndus=…'"></textarea></label>
   {#if curlError}<p class="alert error" role="alert">{curlError}</p>{/if}
   <div class="editor-footer"><button type="button" class="text-btn" onclick={closeCurlImport}>Cancel</button><button class="primary" type="submit">Fill session fields</button></div>
  </form>
 </dialog>
{/if}

<style>
 .tera-auth-dialog,.tera-curl-dialog{width:min(760px,94vw);max-height:90vh;padding:24px;border:1px solid var(--line,#ddd);border-radius:16px;background:var(--surface,#101b2b);color:#edf4fb;overflow:auto}
 .tera-auth-dialog::backdrop,.tera-curl-dialog::backdrop{background:#15151bcc;backdrop-filter:blur(4px)}
 .tera-curl-dialog form,.tera-account-form{display:grid;gap:14px}
 .tera-curl-dialog textarea{width:100%;min-height:220px;resize:vertical;padding:12px;border:1px solid #314d6b;border-radius:7px;background:#0a1423;color:#edf4fb;font:12px/1.5 ui-monospace,monospace}
 .tera-curl-trigger{justify-self:start}
 .tera-account-form{margin:20px 0}.form-row{display:flex;gap:14px;flex-wrap:wrap}.form-row>label{flex:1;min-width:180px}
 label{display:flex;flex-direction:column;gap:6px;font-size:13px;font-weight:600;color:#edf4fb;margin:8px 0}input,select{min-width:0;width:100%;padding:10px;border:1px solid #314d6b;border-radius:7px;background:#0a1423;color:#edf4fb}
 .review-note{color:#cedbe9;font-size:12px}details>p{font-size:13px;line-height:1.7}summary{cursor:pointer}
 @media(max-width:600px){.tera-auth-dialog,.tera-curl-dialog{padding:16px}.form-row>label{min-width:100%}}
</style>
