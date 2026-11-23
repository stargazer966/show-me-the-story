<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { progress, taskRunning, currentProject, addToast, showConfirm } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  let entries=[], selected=null, busy=false, scope='all', chapter=1, part=1;
  async function load(){
    busy=true;
    try {entries=await api('GET','/api/history');}
    catch(e){addToast(e.message,'error')}
    finally{busy=false}
  }
  onMount(load);
  function restore(){
    const entry=selected;
    const target={stage:scope,chapter:Number(chapter),part:Number(part),project:$currentProject};
    showConfirm($t('flow.restoreConfirm',{scope:$t('flow.scope.'+scope),time:new Date(entry.at).toLocaleString()}),async()=>{
      try{
        await api('POST','/api/history/'+entry.id,target);
        window.location.reload();
      }catch(e){addToast(e.message,'error')}
    });
  }
</script>
<section class="card p-4 gap-4">
  <div class="flex items-center justify-between"><h2 class="card-title">{$t('flow.history')}</h2><button class="btn btn-outline btn-sm" disabled={busy} on:click={load}>{$t('desk.reload')}</button></div>
  {#if selected}
    <fieldset class="candidate-panel space-y-2" disabled={$taskRunning||busy}>
      <legend>{new Date(selected.at).toLocaleString()} · {selected.action}</legend>
      <label class="label">{$t('flow.restoreScope')}<select class="select select-sm" bind:value={scope}>
        {#each ['all','settings','draft','outline','prose','proofreading'] as key}<option value={key}>{$t('flow.scope.'+key)}</option>{/each}
      </select></label>
      {#if scope==='prose'||scope==='outline'}<label class="label">{$t('flow.chooseChapter')}<select class="select select-sm" bind:value={chapter}>{#each $progress?.chapters||[] as c}<option value={c.num}>{c.num} · {c.title}</option>{/each}</select></label>{/if}
      {#if scope==='draft'}<label class="label">{$t('flow.partNumber')}<input class="input input-sm w-24" type="number" min="1" bind:value={part}/></label>{/if}
      <button class="btn btn-primary btn-sm" on:click={restore}>{$t('flow.restore')}</button>
    </fieldset>
  {/if}
  {#each entries as entry}
    <button class="btn btn-outline justify-between h-auto py-2 flex-wrap" aria-pressed={selected?.id===entry.id} on:click={()=>selected=entry}>
      <span class="break-all">{entry.action}</span><time datetime={entry.at}>{new Date(entry.at).toLocaleString()}</time>
    </button>
  {:else}
    <p>{busy?$t('desk.working'):$t('flow.historyEmpty')}</p>
  {/each}
</section>
