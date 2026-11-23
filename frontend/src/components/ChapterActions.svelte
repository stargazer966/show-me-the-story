<script>
 import {api} from '../lib/api.js';
 import {progress,settings,taskRunning,selectedChapter,addToast,showConfirm} from '../lib/stores.js';
 import {t} from '../lib/i18n/index.js';
 export let ch;
 async function review(clear){
  try{
   const num=ch.num,url='/api/chapters/'+num;
   const impact=await api('POST',url+'/removal-preview');
   if(clear&&!impact.can_clear){addToast($t('wf.frontier'),'info');return}
   const label=$t(clear?'wf.clear':'wf.remove');
   const message=label+' — '+num+' · '+impact.title+'\n'+$t('wf.impact',{n:clear?0:impact.later,refs:impact.references});
   showConfirm(message,async()=>{try{
    const p=await api('DELETE',url+(clear?'?mode=clear':''),null,{'If-Match':impact.token});
    selectedChapter.set(Math.max(0,Math.min($selectedChapter,p.chapters.length-1)));progress.set(p);
    settings.set(await api('GET','/api/settings'));addToast(label,'success');
   }catch(e){addToast(e.message,'error')}});
  }catch(e){addToast(e.message,'error')}
 }
</script>
{#if !ch.inherited}<div class="flex flex-wrap gap-2">
 {#if ch.status==='accepted'||ch.status==='review'}<button class="btn btn-ghost btn-xs" disabled={$taskRunning} on:click|stopPropagation={()=>review(true)}>{$t('wf.clear')}</button>{/if}
 <button class="btn btn-error btn-outline btn-xs" disabled={$taskRunning} on:click|stopPropagation={()=>review(false)}>{$t('wf.remove')}</button>
</div>{/if}
