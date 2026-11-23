<script>
  import {onMount} from 'svelte';
  import {api} from '../lib/api.js';
  import {currentProject, progress, settings, addToast} from '../lib/stores.js';
  import {t} from '../lib/i18n/index.js';
  import Outline from './Outline.svelte';
  import ImageDraft from '../components/ImageDraft.svelte';
  let desk, contexts=[], chapters=[], index=0, version=0, beat=0, comment='', draft, preview, busy=false, error='', dirty=false, batch=false, cast=false, manual=null, compare=false;
  const project=$currentProject;
  const draftKey=i=>'story-desk:'+JSON.stringify([project,i]);
  async function request(action,extra={}){
    return api('POST','/api/story-desk',{project,action,part:index+1,revision:desk?.revision||0,version,beat,...extra});
  }
  function receive(result){desk=result.desk;chapters=result.chapters||[];contexts=result.contexts||[]}
  function select(i){
    index=i;const part=desk.parts[i];draft={title:part.title,beats:part.beats,focus:part.focus};dirty=false;
    try{const local=JSON.parse(localStorage.getItem(draftKey(i)));if(local){draft=local;dirty=true}}catch{}
    version=chapters.find(c=>c.desk_part===i+1)?.desk_version||part.versions?.length||0;
    beat=0;comment='';preview=null;manual=null;compare=false;
  }
  async function load(){
    busy=true;try{receive(await request('open'));select(Math.min(index,desk.parts.length-1));error=''}catch(e){error=e.message}finally{busy=false}
  }
  onMount(load);
  function edit(field,value){
    draft={...draft,[field]:value};dirty=true;preview=null;
    try{localStorage.setItem(draftKey(index),JSON.stringify(draft))}catch{error=$t('desk.localError')}
  }
  async function save(){
    if(!dirty)return;
    receive(await request('save',{draft}));dirty=false;
    try{localStorage.removeItem(draftKey(index))}catch{}
  }
  async function action(kind,revision=false,mode=''){
    busy=true;error='';try{
      await save();
      if(kind==='save'){addToast($t('common.save'),'success');return}
      if(kind==='preview'||kind==='generate'){
        const response=await request(kind,{beat:revision?beat:0,comment:revision?comment.trim():'',mode});
        if(kind==='preview'){preview=response;return}
        receive(response);version=desk.parts[index].versions.length;beat=0;comment='';
      }else{
        receive(await request(kind,kind==='manual_outline'?{comment:manual}:{}));
        if(kind==='manual_outline'){version=desk.parts[index].versions.length;manual=null}
        if(kind==='add')select(desk.parts.length-1);
        if(kind==='use')progress.set(await api('GET','/api/progress'));
      }
      preview=null;
    }catch(e){error=e.message}finally{busy=false}
  }
  $: part=desk?.parts[index];
  $: candidate=part?.versions?.[version-1];
  $: chapter=chapters.find(c=>c.desk_part===index+1);
  $: adopted=part?.versions?.[(chapter?.desk_version||0)-1];
  $: changed=!!candidate&&(candidate.source?.beats!==draft?.beats||candidate.source?.focus!==draft?.focus||candidate.source?.title!==draft?.title|| (!!candidate.context && candidate.context!==contexts[index]));
</script>
<div class="flex gap-2 mb-4">
  <button class="btn btn-sm" class:btn-primary={!batch} on:click={()=>{if(batch)load();batch=false}}>{$t('nav.outline')}</button>
  <button class="btn btn-sm" class:btn-primary={batch} on:click={()=>batch=true}>{$t('desk.batch')}</button>
</div>
{#if batch}<Outline/>
{:else}
{#if error}<div class="alert alert-error mb-3" role="alert">{error}<button class="btn btn-sm" disabled={busy} on:click={load}>{$t('desk.reload')}</button></div>{/if}
{#if desk}
<div class="flex flex-wrap items-center justify-between gap-2 mb-3">
 <h2 class="text-2xl font-semibold">{$progress?.title || $currentProject}</h2>
 <button class="btn btn-outline btn-sm" on:click={()=>cast=!cast}>{$t('desk.cast')}</button>
</div>
{#if cast}<section class="card bg-base-200 p-4 mb-3">
 <div class="flex flex-wrap gap-2 mb-2">{#each $settings?.characters||[] as c}<span class="badge badge-outline" title={c.appearance}>{c.name}</span>{/each}</div>
 <ImageDraft/><a class="link mt-2" href="#config" on:click={()=>sessionStorage.setItem('story-settings-tab','cast')}>{$t('desk.cast')}</a>
</section>{/if}
<div class="desk">
<nav aria-label={$t('desk.parts')} class="space-y-2"><h3 class="font-semibold">{$t('desk.parts')}</h3>
 {#each desk.parts as item,i}<button disabled={busy} class="btn btn-sm w-full justify-start" class:btn-primary={index===i} on:click={()=>select(i)}>{i+1}. {item.title||$t('desk.untitled')}</button>{/each}
 <button class="btn btn-outline btn-sm" disabled={busy} on:click={()=>action('add')}>＋ {$t('desk.part')}</button>
</nav>
<fieldset disabled={busy} class="card bg-base-200 p-4 space-y-3 min-w-0">
 <h3 class="text-xl font-semibold">{$t('desk.draft')}</h3>
 <label>{$t('desk.title')}<input class="input w-full" value={draft.title} on:input={e=>edit('title',e.target.value)}></label>
 <label>{$t('desk.beats')}<textarea rows="8" class="textarea w-full" value={draft.beats} on:input={e=>edit('beats',e.target.value)}></textarea></label>
 <label>{$t('desk.focus')}<textarea rows="3" class="textarea w-full" value={draft.focus} on:input={e=>edit('focus',e.target.value)}></textarea></label>
 <div class="flex items-center gap-2"><button class="btn btn-outline btn-sm" on:click={()=>action('save')}>{$t('desk.save')}</button><small>{dirty?$t('desk.local'):$t('desk.saved')}</small></div>
</fieldset>
<fieldset disabled={busy} class="card bg-base-200 p-4 space-y-3 min-w-0">
 <h3 class="text-xl font-semibold">{$t('desk.outline')}</h3>
 <div class="flex flex-wrap gap-2"><button class="btn btn-primary btn-sm" disabled={!draft.beats.trim()} on:click={()=>action('generate',false,changed?'update':'rewrite')}>{$t(candidate?(changed?'flow.updateOutline':'flow.rewriteOutline'):'desk.generate')}</button>
 <button class="btn btn-outline btn-sm" disabled={!draft.beats.trim()} on:click={()=>action('preview',false,changed?'update':'rewrite')}>{$t('desk.preview')}</button></div>
 {#if candidate}
 <button class="btn btn-outline btn-sm" on:click={()=>manual=candidate.lines.join('\n\n')}>{$t('flow.editOutline')}</button>
 {#if adopted && adopted!==candidate}<button class="btn btn-ghost btn-sm" on:click={()=>compare=!compare}>{$t('flow.compare')}</button>{/if}
 {#if compare&&adopted}<div class="candidate-diff"><p class="whitespace-pre-wrap">{adopted.lines.join('\n\n')}</p><p class="whitespace-pre-wrap">{candidate.lines.join('\n\n')}</p></div>{/if}
 {#if manual!==null}<textarea class="textarea w-full" rows="7" bind:value={manual} aria-label={$t('flow.editOutline')}></textarea><button class="btn btn-primary btn-sm" disabled={!manual.trim()} on:click={()=>action('manual_outline')}>{$t('common.save')}</button><button class="btn btn-ghost btn-sm" on:click={()=>manual=null}>{$t('common.cancel')}</button>{/if}
 <div class="flex flex-wrap items-center gap-2"><select class="select select-sm" aria-label={$t('desk.version')} bind:value={version} on:change={()=>{beat=0;comment='';preview=null}}>
 {#each part.versions as _,i}<option value={i+1}>{$t('desk.version')} {i+1}{chapter?.desk_version===i+1?' ✓':''}</option>{/each}</select>
 {#if chapter?.desk_version===version}<span class="badge">{$t('desk.active')}</span>{:else}<button class="btn btn-outline btn-sm" on:click={()=>action('use')}>{$t('desk.use')}</button>{/if}</div>
 {#if candidate.source?.beats!==draft.beats||candidate.source?.focus!==draft.focus||candidate.source?.title!==draft.title}<small>{$t('desk.changed')}</small>{/if}
 {#each candidate.lines as line,i}<section class="border-t border-base-300 pt-2"><div class="flex justify-between items-center"><small>{$t('desk.beat')} {i+1}</small><button class="btn btn-ghost btn-xs" on:click={()=>{beat=i+1;preview=null}}>{$t('desk.revise')}</button></div><p class="whitespace-pre-wrap">{line}</p></section>{/each}
 <label>{beat?$t('desk.beat')+' '+beat:$t('desk.outline')} · {$t('desk.comment')}
 <textarea class="textarea w-full" rows="2" bind:value={comment} on:input={()=>preview=null}></textarea></label>
 <div class="flex flex-wrap gap-2"><button class="btn btn-outline btn-sm" disabled={!comment.trim()} on:click={()=>action('generate',true)}>{$t('desk.revise')}</button><button class="btn btn-ghost btn-sm" disabled={!comment.trim()} on:click={()=>action('preview',true)}>{$t('desk.preview')}</button>
 {#if beat}<button class="btn btn-ghost btn-sm" on:click={()=>{beat=0;preview=null}}>{$t('desk.whole')}</button>{/if}</div>
 {#if chapter}<a class="link" href="#writing">{$t('desk.write')}</a>{/if}
 {#if chapter?.desk_earlier_prose && chapter.word_count}<small>{$t('desk.earlier')}</small>{/if}
 {/if}
 {#if busy}<small role="status">{$t('desk.working')}</small>{/if}
</fieldset></div>
{#if preview}<section class="card bg-base-200 p-4 mt-3"><div class="flex justify-between"><h3 class="font-semibold">{$t('desk.preview')}</h3><button class="btn btn-ghost btn-sm" on:click={()=>preview=null}>{$t('common.close')}</button></div>
 {#each ['system','user'] as role}<strong>{role}</strong><pre class="whitespace-pre-wrap break-words text-sm">{preview[role]}</pre>{/each}</section>{/if}
{:else if busy}<p>{$t('desk.working')}</p>{/if}{/if}
