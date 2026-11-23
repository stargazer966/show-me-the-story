<script>
  import {onMount} from 'svelte';
  import {api} from '../lib/api.js';
  import {currentProject, progress, selectedChapter, settings, addToast} from '../lib/stores.js';
  import {t} from '../lib/i18n/index.js';
  import Outline from './Outline.svelte';
  import ImageDraft from '../components/ImageDraft.svelte';
  let desk, contexts=[], chapters=[], index=0, version=0, beat=0, comment='', draft, preview, busy=false, error='', dirty=false, batch=false, cast=false, manual=null, compare=false, rows=[];
  const project=$currentProject;
  const draftKey=i=>'story-desk:'+JSON.stringify([project,i]);
  async function request(action,extra={}){
    return api('POST','/api/story-desk',{project,action,part:index+1,revision:desk?.revision||0,version,beat,...extra});
  }
  function receive(result){
    desk=result.desk;chapters=result.chapters||[];contexts=result.contexts||[];
    if(result.progress){progress.set(result.progress);focusWritingChapter(chapters)}
  }
  function select(i){
    index=i;const part=desk.parts[i];draft={title:part.title,beats:part.beats,focus:part.focus,remarks:part.remarks||[]};dirty=false;
    try{const local=JSON.parse(localStorage.getItem(draftKey(i)));if(local){draft=local;dirty=true}}catch{}
    const lines=(draft.beats||"").split("\n");
    rows=(draft.remarks?.length?lines:lines.filter(v=>v.trim())).map((text,n)=>({text,remark:draft.remarks?.[n]||""}));
    if(!rows.length)rows=[{text:"",remark:""}];
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
  function setRows(next){
    rows=next;
    const filled=rows.filter(row=>row.text.trim()||row.remark.trim());
    const remarks=filled.map(row=>row.remark.trim());
    while(remarks.length && !remarks[remarks.length-1])remarks.pop();
    draft={...draft,beats:filled.map(row=>row.text.replace(/\r?\n/g,' ').trim()).join('\n'),remarks};
    edit('beats',draft.beats);
  }
  function editRow(i,field,value){
    setRows(rows.map((row,n)=>n===i?{...row,[field]:value}:row));
  }
  function beginEdit(){
    manual=candidate.lines.map((line,i)=>({line,instruction:candidate.instructions?.[i]||''}));
  }
  function editOutput(i,field,value){
    manual=manual.map((entry,n)=>n===i?{...entry,[field]:value}:entry);
  }
  async function save(){
    if(!dirty)return;
    receive(await request('save',{draft}));dirty=false;
    try{localStorage.removeItem(draftKey(index))}catch{}
  }
  async function action(kind,revision=false,mode=''){
    if(incomplete){error=$t('desk.missingBeat');return}
    busy=true;error='';try{
      await save();
      if(kind==='save'){addToast($t('common.save'),'success');return}
      if(kind==='preview'||kind==='generate'){
        const response=await request(kind,{beat:revision?beat:0,comment:revision?comment.trim():'',mode});
        if(kind==='preview'){preview=response;return}
        receive(response);version=desk.parts[index].versions.length;beat=0;comment='';
      }else{
        const result=await request(kind,kind==='manual_outline'?{lines:manual.map(v=>v.line),instructions:manual.map(v=>v.instruction)}:{});
        receive(result);
        if(kind==='manual_outline'){version=desk.parts[index].versions.length;manual=null}
        if(kind==='add')select(desk.parts.length-1);
      }
      preview=null;
    }catch(e){error=e.message}finally{busy=false}
  }
  function partName(part,i){
    const adopted=chapters.find(ch=>ch.desk_part===i+1)?.desk_version;
    return part.title || part.versions?.[(adopted || part.versions.length)-1]?.title || $t('desk.untitled');
  }
  function focusWritingChapter(list){
    const target=list.findIndex(ch=>ch.desk_part===index+1);
    if(target>=0)selectedChapter.set(target);
  }
  $: part=desk?.parts[index];
  $: candidate=part?.versions?.[version-1];
  $: chapter=chapters.find(c=>c.desk_part===index+1);
  $: adopted=part?.versions?.[(chapter?.desk_version||0)-1];
  $: incomplete=rows.some(r=>!r.text.trim()&&r.remark.trim());
  $: changed=!!candidate&&(candidate.source?.beats!==draft?.beats||candidate.source?.focus!==draft?.focus||candidate.source?.title!==draft?.title||JSON.stringify(candidate.source?.remarks||[])!==JSON.stringify(draft?.remarks||[])|| (!!candidate.context && candidate.context!==contexts[index]));
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
<div class="desk desk-beat-layout">
<nav aria-label={$t('desk.parts')} class="space-y-2"><h3 class="font-semibold">{$t('desk.parts')}</h3>
 {#each desk.parts as item,i}<button disabled={busy} class="btn btn-sm w-full justify-start" class:btn-primary={index===i} on:click={()=>select(i)}>{i+1}. {partName(item,i)}</button>{/each}
 <button class="btn btn-outline btn-sm" disabled={busy} on:click={()=>action('add')}>＋ {$t('desk.part')}</button>
</nav>
<fieldset disabled={busy} class="card bg-base-200 p-4 space-y-3 min-w-0">
 <h3 class="text-xl font-semibold">{$t('desk.draft')}</h3>
 <label>{$t('desk.title')}<input class="input w-full" value={draft.title} placeholder={candidate?.title||$t('desk.untitled')} on:input={e=>edit('title',e.target.value)}></label>
 <label>{$t('desk.chapterRemarks')}<textarea rows="3" class="textarea w-full" value={draft.focus} on:input={e=>edit('focus',e.target.value)}></textarea></label>
 <div class="flex items-center gap-2"><button class="btn btn-outline btn-sm" on:click={()=>action('save')}>{$t('desk.save')}</button><small>{dirty?$t('desk.local'):$t('desk.saved')}</small></div>
</fieldset>
<fieldset disabled={busy} class="card bg-base-200 p-4 space-y-3 min-w-0">
 <h3 class="text-xl font-semibold">{$t('desk.outline')}</h3>
 <div class="flex flex-wrap gap-2"><button class="btn btn-primary btn-sm" disabled={!draft.beats.trim()||incomplete} on:click={()=>action('generate',false,changed?'update':'rewrite')}>{$t(candidate?(changed?'flow.updateOutline':'flow.rewriteOutline'):'desk.generate')}</button>
 <button class="btn btn-outline btn-sm" disabled={!draft.beats.trim()||incomplete} on:click={()=>action('preview',false,changed?'update':'rewrite')}>{$t('desk.preview')}</button></div>
 {#if candidate}
 {#if candidate.title}<h4 class="font-semibold">{candidate.source?.title||candidate.title}</h4>{/if}
 <button class="btn btn-outline btn-sm" on:click={beginEdit}>{$t('desk.editOutputs')}</button>
 {#if adopted && adopted!==candidate}<button class="btn btn-ghost btn-sm" on:click={()=>compare=!compare}>{$t('flow.compare')}</button>{/if}
 {#if compare&&adopted}<div class="candidate-diff">{#each [adopted,candidate] as entry}<div>{#each entry.lines as line,i}<p>{line}</p>{#if entry.instructions?.[i]}<p class="mb-2 font-medium">{entry.instructions[i]}</p>{/if}{/each}</div>{/each}</div>{/if}
 {#if manual}<button class="btn btn-primary btn-sm" disabled={manual.some(v=>!v.line.trim())} on:click={()=>action('manual_outline')}>{$t('common.save')}</button><button class="btn btn-ghost btn-sm" on:click={()=>manual=null}>{$t('common.cancel')}</button>{/if}
 <div class="flex flex-wrap items-center gap-2"><select class="select select-sm" aria-label={$t('desk.version')} bind:value={version} on:change={()=>{beat=0;comment='';preview=null;manual=null}}>
 {#each part.versions as _,i}<option value={i+1}>{$t('desk.version')} {i+1}{chapter?.desk_version===i+1?' ✓':''}</option>{/each}</select>
 {#if chapter?.desk_version===version}<span class="badge">{$t('desk.active')}</span>{:else}<button class="btn btn-outline btn-sm" on:click={()=>action('use')}>{$t('desk.use')}</button>{/if}</div>
 {#if changed}<small>{$t('desk.changed')}</small>{/if}
 <label>{beat?$t('desk.beat')+' '+beat:$t('desk.outline')} · {$t('desk.comment')}
 <textarea class="textarea w-full" rows="2" bind:value={comment} on:input={()=>preview=null}></textarea></label>
 <div class="flex flex-wrap gap-2"><button class="btn btn-outline btn-sm" disabled={!comment.trim()} on:click={()=>action('generate',true)}>{$t('desk.revise')}</button><button class="btn btn-ghost btn-sm" disabled={!comment.trim()} on:click={()=>action('preview',true)}>{$t('desk.preview')}</button>
 {#if beat}<button class="btn btn-ghost btn-sm" on:click={()=>{beat=0;preview=null}}>{$t('desk.whole')}</button>{/if}</div>
 {#if chapter}<a class="link" href="#writing" on:click={()=>focusWritingChapter(chapters)}>{$t('desk.write')}</a>{/if}
 {#if chapter?.desk_earlier_prose && chapter.word_count}<small>{$t('desk.earlier')}</small>{/if}
 {/if}
 {#if busy}<small role="status">{$t('desk.working')}</small>{/if}
</fieldset>
<fieldset class="desk-beat-cards min-w-0 space-y-3" disabled={busy}>
 {#if incomplete}<p role="status">{$t('desk.missingBeat')}</p>{/if}
 {#each Array(Math.max(rows.length,candidate?.lines?.length||0)) as _,i}
  <details class="card bg-base-200 p-4" open={!!manual||!candidate||i===0}>
   <summary class="cursor-pointer font-semibold">{$t('desk.beat')} {i+1}{#if rows[i]?.text} · <span class="font-normal">{rows[i].text.slice(0,65)}</span>{/if}</summary>
   <div class="desk-beat-fields mt-3">
    <div class="min-w-0 space-y-2">
     {#if rows[i]}
      <label>{$t('desk.beatDraft')}<textarea class="textarea w-full" rows="3" value={rows[i].text} on:input={e=>editRow(i,'text',e.target.value)}></textarea></label>
      <label>{$t('desk.beatRemarks')}<textarea class="textarea w-full" rows="3" value={rows[i].remark} on:input={e=>editRow(i,'remark',e.target.value)}></textarea></label>
      <button class="btn btn-ghost btn-xs" on:click={()=>setRows(rows.length===1?[{text:'',remark:''}]:rows.filter((_,n)=>n!==i))}>{$t('desk.removeBeat')}</button>
     {/if}
    </div>
    <div class="min-w-0 space-y-2">
     {#if manual?.[i]}
      <label>{$t('desk.beatOutline')}<textarea class="textarea w-full" rows="3" value={manual[i].line} on:input={e=>editOutput(i,'line',e.target.value)}></textarea></label>
      <label>{$t('desk.instructions')}<textarea class="textarea w-full" rows="3" value={manual[i].instruction} on:input={e=>editOutput(i,'instruction',e.target.value)}></textarea></label>
     {:else if candidate?.lines?.[i]}
      <h4 class="font-medium">{$t('desk.beatOutline')}</h4><p class="whitespace-pre-wrap break-words">{candidate.lines[i]}</p>
      <h4 class="font-medium">{$t('desk.instructions')}</h4><p class="whitespace-pre-wrap break-words">{candidate.instructions?.[i]||'—'}</p>
      <button class="btn btn-ghost btn-xs" on:click={()=>{beat=i+1;preview=null}}>{$t('desk.revise')}</button>
     {/if}
    </div>
   </div>
  </details>
 {/each}
 <button class="btn btn-outline btn-sm" on:click={()=>setRows([...rows,{text:'',remark:''}])}>＋ {$t('desk.addBeat')}</button>
</fieldset></div>
{#if preview}<section class="card bg-base-200 p-4 mt-3"><div class="flex justify-between"><h3 class="font-semibold">{$t('desk.preview')}</h3><button class="btn btn-ghost btn-sm" on:click={()=>preview=null}>{$t('common.close')}</button></div>
 {#each ['system','user'] as role}<strong>{role}</strong><pre class="whitespace-pre-wrap break-words text-sm">{preview[role]}</pre>{/each}</section>{/if}
{:else if busy}<p>{$t('desk.working')}</p>{/if}{/if}
