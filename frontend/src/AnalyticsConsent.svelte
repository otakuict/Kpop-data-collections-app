<script>
 import { X, ShieldCheck } from '@lucide/svelte';
 let { choice, onchange } = $props();
 let settings=$state(false);
 export function openSettings(){settings=true;}
 function choose(next){onchange(next);settings=false;}
 function modal(node){node.showModal();return {destroy(){node.close();}};}
</script>

{#if choice===null&&!settings}
 <section class="consent-banner" aria-label="Analytics cookies">
  <div class="consent-copy"><h2><ShieldCheck size={18}/>Your choice, your visit.</h2><p>With your permission, we use a browser cookie and collect visit data to understand how the gallery is used. You can enjoy the gallery without analytics.</p></div>
  <div class="consent-actions"><button class="consent-button" onclick={()=>choose('accepted')}>Accept analytics</button><button class="consent-button" onclick={()=>choose('rejected')}>Reject analytics</button><button class="privacy-link" onclick={openSettings}>Privacy & cookies</button></div>
 </section>
{/if}

{#if settings}
 <dialog class="cookie-settings" use:modal aria-labelledby="cookie-title" oncancel={()=>settings=false}>
  <button class="cookie-close" aria-label="Close cookie settings" onclick={()=>settings=false}><X size={20}/></button>
  <div class="eyebrow">YOUR PRIVACY</div><h2 id="cookie-title">Privacy & cookies</h2>
  <p class="cookie-status">{choice==='accepted'?'Analytics is on':choice==='rejected'?'Analytics is off':'Analytics is off until you accept'}</p>
  <h3>Optional gallery analytics</h3>
  <p>If you accept, a random browser identifier in the <code>gallery_visitor</code> cookie helps count people and visits. A heartbeat every 20 seconds estimates time spent while the page is visible. A 60-second gap starts a new visit.</p>
  <p>We collect and store information about gallery visits for site statistics. We do not store your name or IP address, and we do not use this information for advertising or tracking across other sites.</p>
  <h3>Storage and your choice</h3>
  <p>The visitor cookie lasts for your browser session. We keep visit records for 90 days; expired records are removed when analytics next runs or the dashboard is opened. Hosting services may retain separate backups and request logs.</p>
  <p>We remember your accept or reject choice in this browser for 180 days. You can change it anytime using <strong>Cookie settings</strong> in the footer. Rejecting stops future analytics and clears the visitor cookie. Records collected before withdrawal remain until the retention period ends.</p>
  <h3>Browsing without analytics</h3>
  <p>Search, filters and images work without analytics. Your cookie choice, short-guide preference and administrator sign-in use separate browser storage for those features. Online counts include only visitors who accept analytics.</p>
  <p>For privacy questions, contact the site operator through the gallery’s contact links.</p>
  <div class="settings-actions"><button class="consent-button" onclick={()=>choose('accepted')}>Accept analytics</button><button class="consent-button" onclick={()=>choose('rejected')}>Reject analytics</button></div>
 </dialog>
{/if}

<style>
 .consent-banner{position:fixed;bottom:20px;left:50%;transform:translateX(-50%);z-index:8;display:flex;align-items:center;gap:28px;width:min(1020px,calc(100% - 40px));padding:24px 28px;background:#122338;border:1px solid #41627f;border-radius:12px;box-shadow:0 14px 55px #0009}
 .consent-copy{flex:1;min-width:0}
 .consent-copy h2{display:flex;align-items:center;gap:9px;font-size:16px;font-weight:600;margin:0 0 9px}
 .consent-copy h2 :global(svg){color:#75d7f2}
 .consent-copy p{font-size:12px;line-height:1.8;color:#b3c8dc;margin:0;max-width:620px}
 .consent-actions{display:grid;grid-template-columns:1fr 1fr;gap:10px;flex-shrink:0}
 .consent-button{min-height:44px;display:flex;align-items:center;justify-content:center;padding:11px 15px;border:1px solid #5987a6;border-radius:6px;background:#1b3c54;color:#d9f3ff;font-size:12px;font-weight:550;white-space:nowrap}
 .consent-button:hover{background:#295572;color:#fff}
 .privacy-link{grid-column:1/-1;text-decoration:underline;text-underline-offset:4px;font-size:11px;color:#adc7dd;min-height:28px;justify-self:center}
 .cookie-settings{display:block;width:min(610px,calc(100vw - 32px));padding:34px;background:#101f30;border:1px solid #3f607b;border-radius:12px;color:#edf4fb;max-height:calc(100dvh - 32px)}
 .cookie-settings::backdrop{background:#000a;backdrop-filter:blur(5px)}
 .cookie-settings h2{font-family:Manrope,sans-serif;font-size:27px;letter-spacing:-.8px;margin:13px 35px 18px 0}
 .cookie-settings h3{font-size:13px;margin:24px 0 10px;font-weight:600}
 .cookie-settings p{font-size:12px;color:#b3c8dc;line-height:1.9;margin:10px 0}
 .cookie-settings .cookie-status{padding:10px 13px;background:#17394b;border:1px solid #31556e;border-radius:5px;color:#a6e8f6;font-size:12px}
 .cookie-settings code{font-size:11px;color:#d4e9f8}
 .cookie-close{position:absolute;right:20px;top:20px;display:flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:50%;background:#213b52;color:#d2e9f9}
 .settings-actions{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:25px;padding-top:20px;border-top:1px solid #30485f}
 @media(max-width:700px){.consent-banner{bottom:12px;width:calc(100% - 24px);padding:20px;flex-direction:column;gap:15px;align-items:stretch}.consent-copy h2{font-size:15px}.consent-copy p{font-size:11px}.consent-actions{gap:9px}.consent-button{padding:11px;font-size:12px}.cookie-settings{padding:27px 22px}.cookie-settings h2{font-size:24px}.settings-actions{gap:10px}}
</style>
