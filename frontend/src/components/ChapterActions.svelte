<script>
 import {api} from '../lib/api.js';
 import {progress,settings,taskRunning,selectedChapter,addToast,showConfirm} from '../lib/stores.js';
 import {t} from '../lib/i18n/index.js';
 export let ch;
 async function review(){
  try{
   const num=ch.num,url='/api/chapters/'+num;
   const impact=await api('POST',url+'/removal-preview');
   if(!impact.can_clear)return;
   const label=$t('wf.clear');
   const message=label+' — '+num+' · '+impact.title+'\n'+$t('wf.impact',{n:0,refs:impact.references});
   showConfirm(message,async()=>{try{
    const p=await api('DELETE',url+'?mode=clear',null,{'If-Match':impact.token});
    selectedChapter.set(Math.max(0,Math.min($selectedChapter,p.chapters.length-1)));progress.set(p);
    settings.set(await api('GET','/api/settings'));addToast(label,'success');
   }catch(e){addToast(e.message,'error')}});
  }catch(e){addToast(e.message,'error')}
 }
</script>
{#if !ch.inherited&&(ch.status==='accepted'||ch.status==='review')}<details class="dropdown dropdown-end shrink-0"><summary class="btn btn-ghost btn-sm" aria-label={$t('flow.chapterActions')}>⋯</summary><div class="dropdown-content bg-base-200 border border-base-300 rounded p-2 z-20">
 {#if ch.status==='accepted'||ch.status==='review'}<button class="btn btn-ghost btn-xs" disabled={$taskRunning} on:click|stopPropagation={review}>{$t('wf.clear')}</button>{/if}

</div></details>{/if}
