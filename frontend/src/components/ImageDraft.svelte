<script>
 import {onDestroy} from 'svelte';
 import {api,apiFetch} from '../lib/api.js';
 import {settings,addToast,taskRunning} from '../lib/stores.js';
 import {t} from '../lib/i18n/index.js';
 let file, preview='', panel='', result=null, name='', busy=false, savedChar=false, savedScene=false;
 function select(next){if(busy||!next)return;if(preview)URL.revokeObjectURL(preview);file=next;preview=URL.createObjectURL(file);result=null;name='';}
 onDestroy(()=>{if(preview)URL.revokeObjectURL(preview)});
 async function analyze(){
   busy=true;
   try{const data=new FormData();data.append('image',file);data.append('panel',panel);
     result=await (await apiFetch('/api/image-draft',{method:'POST',body:data})).json();savedChar=savedScene=false;
   }catch(e){addToast(e.message,'error')}finally{busy=false}
 }
 async function save(kind){
   busy=true;
   try{
     if(kind==='character'){await api('POST','/api/characters',{name:name.trim(),appearance:result.appearance});savedChar=true}
     else{await api('POST','/api/worldview',{name:result.location_name.trim(),category:'geography',description:result.scenery});savedScene=true}
     settings.set(await api('GET','/api/settings'));addToast($t('common.save'),'success')
   }catch(e){addToast(e.message,'error')}finally{busy=false}
 }
</script>
<details class="rounded-lg border border-base-300 p-3" on:dragover|preventDefault on:drop|preventDefault={e=>select(e.dataTransfer.files[0])}>
 <summary class="cursor-pointer font-medium">{$t('wf.image')}</summary>
 <div class="grid gap-3 mt-3 md:grid-cols-2">
 <div class="space-y-2"><input type="file" class="file-input w-full" accept="image/png,image/jpeg,image/webp" disabled={busy} on:change={e=>select(e.currentTarget.files[0])}/>
 {#if preview}<img class="max-h-72 object-contain" src={preview} alt={$t('wf.image')}/>{/if}
 <label>{$t('wf.panel')} <select class="select" bind:value={panel} disabled={busy}>{#each ['','top-left','top-right','bottom-left','bottom-right'] as pos}<option value={pos}>{pos||$t('wf.whole')}</option>{/each}</select></label>
 <button class="btn btn-outline" disabled={!file||busy||$taskRunning} on:click={analyze}>{$t('wf.analyze')}</button></div>
 {#if result}<div class="space-y-2"><input class="input w-full" bind:value={name} placeholder={$t('config.char.title')} aria-label={$t('config.char.title')}/>
 <textarea class="textarea w-full" bind:value={result.appearance} aria-label={$t('facts.field.appearance')}></textarea>
 <button class="btn btn-outline btn-sm" disabled={busy||savedChar||$taskRunning||!name.trim()||!result.appearance.trim()} on:click={()=>save('character')}>{$t('wf.saveCharacter')}</button>
 <input class="input w-full" bind:value={result.location_name} aria-label={$t('wf.scenes')}/>
 <textarea class="textarea w-full" bind:value={result.scenery} aria-label={$t('wf.scenes')}></textarea>
 <button class="btn btn-outline btn-sm" disabled={busy||savedScene||$taskRunning||!result.location_name.trim()||!result.scenery.trim()} on:click={()=>save('scene')}>{$t('wf.saveScene')}</button></div>{/if}
 </div>
</details>
