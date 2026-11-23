<script>
  import {onMount,tick} from 'svelte';
  import {api} from '../lib/api.js';
  import {currentProject, progress, selectedChapter, settings, addToast} from '../lib/stores.js';
  import {t} from '../lib/i18n/index.js';
  import Outline from './Outline.svelte';
  import ImageDraft from '../components/ImageDraft.svelte';
  let desk, contexts=[], chapters=[], index=0, version=0, beat=0, comment='', draft, preview, busy=false, error='', dirty=false, batch=false, cast=false, manual=null, compare=false, rows=[];
  let openBeat=0, showRevision=false, revisionInput;
  const project=$currentProject;

  async function openRevision(target=0){
    beat=target;
    showRevision=true;
    preview=null;
    await tick();
    revisionInput?.focus();
  }

  async function addBeat(){
    openBeat=rows.length;
    setRows([...rows,{text:'',remark:''}]);
    await tick();
    document.getElementById('desk-beat-'+openBeat)?.querySelector('textarea')?.focus();
  }

  function removeBeat(i){
    setRows(rows.length===1?[{text:'',remark:''}]:rows.filter((_,n)=>n!==i));
    openBeat=Math.min(i,rows.length-1);
  }
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
    beat=0;comment='';preview=null;manual=null;compare=false;openBeat=0;showRevision=false;
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
    showRevision=false;
    if(openBeat<0 || openBeat>=candidate.lines.length)openBeat=0;
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
        receive(response);version=desk.parts[index].versions.length;beat=0;comment='';showRevision=false;
      }else{
        const result=await request(kind,kind==='manual_outline'?{lines:manual.map(v=>v.line),instructions:manual.map(v=>v.instruction)}:{});
        receive(result);
        if(kind==='manual_outline'){version=desk.parts[index].versions.length;manual=null}
        if(kind==='add')select(desk.parts.length-1);
      }
      preview=null;
    }catch(e){error=e.message}finally{busy=false}
  }
  function partName(part,i,list,untitled){
    const adopted=list.find(ch=>ch.desk_part===i+1)?.desk_version;
    return part.title || part.versions?.[(adopted || part.versions.length)-1]?.title || untitled;
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

<section class="story-desk">
  <div class="flex flex-wrap items-center justify-between gap-2">
    <div class="join">
      <button class="btn btn-sm join-item" class:btn-primary={!batch} aria-pressed={!batch} on:click={()=>{if(batch)load();batch=false}}>{$t('nav.outline')}</button>
      <button class="btn btn-sm join-item" class:btn-primary={batch} aria-pressed={batch} on:click={()=>batch=true}>{$t('desk.batch')}</button>
    </div>
    {#if desk&&!batch}<button class="btn btn-ghost btn-sm" aria-expanded={cast} on:click={()=>cast=!cast}>{$t('desk.cast')}</button>{/if}
  </div>

  {#if batch}
    <Outline/>
  {:else}
    {#if error}<div class="alert alert-error" role="alert">{error}<button class="btn btn-sm" disabled={busy} on:click={load}>{$t('desk.reload')}</button></div>{/if}
    {#if desk}
      {#if cast}
        <section class="card bg-base-200 p-4">
          <div class="flex flex-wrap gap-2 mb-2">{#each $settings?.characters||[] as c}<span class="badge badge-outline" title={c.appearance}>{c.name}</span>{/each}</div>
          <ImageDraft/>
          <a class="link mt-2" href="#config" on:click={()=>sessionStorage.setItem('story-settings-tab','cast')}>{$t('desk.cast')}</a>
        </section>
      {/if}

      <header class="desk-header">
        <div class="desk-toolbar">
          <div class="desk-part-select">
            <select class="select select-sm w-full" aria-label={$t('desk.parts')} value={index} disabled={busy||!!manual} on:change={e=>select(Number(e.currentTarget.value))}>
              {#each desk.parts as item,i}<option value={i}>{i+1}. {partName(item,i,chapters,$t('desk.untitled'))}</option>{/each}
            </select>
            <button class="btn btn-outline btn-sm" disabled={busy||!!manual} on:click={()=>action('add')}>＋ {$t('desk.part')}</button>
          </div>
          <div class="desk-actions">
            {#if manual}
              <button class="btn btn-primary btn-sm" disabled={busy||manual.some(row=>!row.line.trim())} on:click={()=>action('manual_outline')}>{$t('common.save')}</button>
              <button class="btn btn-ghost btn-sm" disabled={busy} on:click={()=>manual=null}>{$t('common.cancel')}</button>
            {:else}
              <button class="btn btn-outline btn-sm" disabled={busy||!dirty} on:click={()=>action('save')}>{$t('desk.save')}</button>
              <button class="btn btn-primary btn-sm" disabled={busy||!draft.beats.trim()||incomplete} on:click={()=>action('generate',false,changed?'update':'rewrite')}>{$t(candidate?(changed?'flow.updateOutline':'flow.rewriteOutline'):'desk.generate')}</button>
              {#if chapter?.outline}<a class="btn btn-ghost btn-sm" href="#writing" on:click={()=>focusWritingChapter(chapters)}>{$t('desk.write')}</a>{/if}
            {/if}
          </div>
        </div>
        <div class="desk-toolbar desk-toolbar-secondary">
          <div class="desk-actions">
            <details class="desk-menu">
              <summary class="btn btn-ghost btn-sm">{$t('desk.chapterSettings')}</summary>
              <fieldset disabled={busy} class="desk-menu-content space-y-3">
                <label>{$t('desk.title')}<input class="input w-full" value={draft.title} placeholder={candidate?.title||$t('desk.untitled')} on:input={e=>edit('title',e.target.value)}></label>
                <label>{$t('desk.chapterRemarks')}<textarea rows="3" class="textarea w-full" value={draft.focus} on:input={e=>edit('focus',e.target.value)}></textarea></label>
              </fieldset>
            </details>
            {#if candidate}
              <select class="select select-sm desk-version" aria-label={$t('desk.version')} bind:value={version} disabled={busy||!!manual} on:change={()=>{beat=0;comment='';preview=null;showRevision=false}}>
                {#each part.versions as _,i}<option value={i+1}>{$t('desk.version')} {i+1}{chapter?.desk_version===i+1?' ✓':''}</option>{/each}
              </select>
              {#if chapter?.desk_version===version}<small class="desk-active">{$t('desk.active')}</small>
              {:else}<button class="btn btn-outline btn-sm" disabled={busy||!!manual} on:click={()=>action('use')}>{$t('desk.use')}</button>{/if}
            {/if}
          </div>
          <div class="desk-actions">
            {#if candidate&&!manual}
              <button class="btn btn-ghost btn-sm" disabled={busy} on:click={beginEdit}>{$t('desk.editOutputs')}</button>
              <button class="btn btn-ghost btn-sm" disabled={busy} aria-expanded={showRevision} on:click={()=>showRevision?showRevision=false:openRevision()}>{$t('desk.reviseOutline')}</button>
              {#if adopted&&adopted!==candidate}<button class="btn btn-ghost btn-sm" aria-expanded={compare} on:click={()=>compare=!compare}>{$t('flow.compare')}</button>{/if}
            {/if}
            {#if !manual}<button class="btn btn-ghost btn-sm" disabled={busy||!draft.beats.trim()||incomplete} on:click={()=>preview?preview=null:action('preview',false,changed?'update':'rewrite')}>{$t('desk.preview')}</button>{/if}
          </div>
        </div>
        {#if busy||dirty||changed||incomplete||chapter?.desk_earlier_prose&&chapter.word_count}
          <div class="desk-notices" role="status">
            {#if busy}<span>{$t('desk.working')}</span>{/if}
            {#if dirty}<span>{$t('desk.local')}</span>{:else if changed}<span>{$t('desk.changed')}</span>{/if}
            {#if incomplete}<span>{$t('desk.missingBeat')}</span>{/if}
            {#if chapter?.desk_earlier_prose&&chapter.word_count}<span>{$t('desk.earlier')}</span>{/if}
          </div>
        {/if}
      </header>

      {#if showRevision&&!manual}
        <fieldset class="desk-inspector" disabled={busy}>
          <label>{beat?$t('desk.beat')+' '+beat:$t('desk.outline')} · {$t('desk.comment')}
            <textarea class="textarea w-full" rows="2" bind:value={comment} bind:this={revisionInput} on:input={()=>preview=null}></textarea>
          </label>
          <div class="desk-actions">
            <button class="btn btn-outline btn-sm" disabled={!comment.trim()} on:click={()=>action('generate',true)}>{$t('desk.revise')}</button>
            <button class="btn btn-ghost btn-sm" disabled={!comment.trim()} on:click={()=>action('preview',true)}>{$t('desk.preview')}</button>
            {#if beat}<button class="btn btn-ghost btn-sm" on:click={()=>openRevision()}>{$t('desk.whole')}</button>{/if}
            <button class="btn btn-ghost btn-sm" on:click={()=>showRevision=false}>{$t('common.close')}</button>
          </div>
        </fieldset>
      {/if}
      {#if preview}
        <section class="desk-inspector">
          <div class="flex justify-between gap-2"><h3 class="font-semibold">{$t('desk.preview')}</h3><button class="btn btn-ghost btn-sm" on:click={()=>preview=null}>{$t('common.close')}</button></div>
          {#each ['system','user'] as role}<strong>{role}</strong><pre class="whitespace-pre-wrap break-words text-sm">{preview[role]}</pre>{/each}
        </section>
      {/if}
      {#if compare&&adopted&&candidate!==adopted}
        <div class="candidate-diff">
          {#each [adopted,candidate] as entry}<div>{#each entry.lines as line,i}<p>{line}</p>{#if entry.instructions?.[i]}<p class="mb-2 font-medium">{entry.instructions[i]}</p>{/if}{/each}</div>{/each}
        </div>
      {/if}

      <fieldset class="desk-beat-cards min-w-0" disabled={busy}>
        <legend class="sr-only">{$t('desk.beat')}</legend>
        {#each Array(Math.max(rows.length,candidate?.lines?.length||0)) as _,i}
          <section class="desk-beat">
            <button class="desk-beat-toggle" aria-expanded={openBeat===i} aria-controls={'desk-beat-'+i} on:click={()=>openBeat=openBeat===i?-1:i}>
              <strong>{$t('desk.beat')} {i+1}</strong>
              <span>{rows[i]?.text||candidate?.lines?.[i]||''}</span>
              <span aria-hidden="true">{openBeat===i?'▾':'▸'}</span>
            </button>
            {#if openBeat===i}
              <div id={'desk-beat-'+i} class="desk-beat-fields">
                <div class="desk-inputs">
                  {#if rows[i]}
                    <label>{$t('desk.beatDraft')}<textarea class="textarea w-full" rows="3" value={rows[i].text} on:input={e=>editRow(i,'text',e.target.value)}></textarea></label>
                    <label>{$t('desk.beatRemarks')}<textarea class="textarea w-full" rows="2" value={rows[i].remark} on:input={e=>editRow(i,'remark',e.target.value)}></textarea></label>
                    <button class="btn btn-ghost btn-xs" disabled={!!manual} on:click={()=>removeBeat(i)}>{$t('desk.removeBeat')}</button>
                  {/if}
                </div>
                {#if manual?.[i]}
                  <div class="desk-outputs">
                    <label>{$t('desk.beatOutline')}<textarea class="textarea w-full" rows="4" value={manual[i].line} on:input={e=>editOutput(i,'line',e.target.value)}></textarea></label>
                    <label>{$t('desk.instructions')}<textarea class="textarea w-full" rows="2" value={manual[i].instruction} on:input={e=>editOutput(i,'instruction',e.target.value)}></textarea></label>
                  </div>
                {:else if candidate?.lines?.[i]}
                  <div class="desk-outputs">
                    <h4>{$t('desk.beatOutline')}</h4><p class="whitespace-pre-wrap break-words">{candidate.lines[i]}</p>
                    <h4>{$t('desk.instructions')}</h4><p class="whitespace-pre-wrap break-words">{candidate.instructions?.[i]||'—'}</p>
                    <button class="btn btn-ghost btn-xs" on:click={()=>openRevision(i+1)}>{$t('desk.revise')}</button>
                  </div>
                {/if}
              </div>
            {/if}
          </section>
        {/each}
        <button class="btn btn-outline btn-sm justify-self-start" disabled={!!manual} on:click={addBeat}>＋ {$t('desk.addBeat')}</button>
      </fieldset>
    {:else if busy}
      <p role="status">{$t('desk.working')}</p>
    {/if}
  {/if}
</section>
