<script>
  import {onMount,tick} from 'svelte';
  import {api} from '../lib/api.js';
  import {currentProject, progress, selectedChapter, settings, addToast, showConfirm} from '../lib/stores.js';
  import {t} from '../lib/i18n/index.js';
  import Outline from './Outline.svelte';
  import DeskSettingSuggestions from '../components/DeskSettingSuggestions.svelte';
  import ImageDraft from '../components/ImageDraft.svelte';
  let desk, contexts=[], chapters=[], index=0, version=0, comment='', draft, busy=false, error='', dirty=false, batch=false, cast=false, manual=null, compare=false, rows=[];
  let openBeats=new Set([0]), revisionBeat=null, revisionInput, chapterSettings;
  const project=$currentProject;
  let settingDecisions = {};

  async function openRevision(target){
    revisionBeat=target;
    comment='';
    await tick();
    revisionInput?.focus();
  }

  function toggleBeat(i){
    const next=new Set(openBeats);
    if(next.has(i))next.delete(i);
    else next.add(i);
    openBeats=next;
  }

  async function addBeat(){
    const i=rows.length;
    openBeats=new Set([...openBeats,i]);
    setRows([...rows,{text:'',remark:''}]);
    await tick();
    document.getElementById('desk-beat-'+i)?.querySelector('textarea')?.focus();
  }

  function removeBeat(i){
    setRows(rows.length===1?[{text:'',remark:''}]:rows.filter((_,n)=>n!==i));
    revisionBeat=null;
    const remaining=[...openBeats].filter(n=>n!==i).map(n=>n>i?n-1:n);
    openBeats=new Set([...remaining,Math.min(i,rows.length-1)]);
  }
  const draftKey=i=>'story-desk:'+JSON.stringify([project,i]);
  async function request(action,extra={}){
    return api('POST','/api/story-desk',{project,action,part:index+1,revision:desk?.revision||0,version,...extra});
  }
  function receive(result){
    settingDecisions = result.setting_decisions || {};
    desk=result.desk;chapters=result.chapters||[];contexts=result.contexts||[];
    if(result.progress){progress.set(result.progress);focusWritingChapter(chapters)}
  }
  function selectAvailable(preferred){
    const available=desk.parts.map((_,i)=>i).filter(i=>!desk.parts[i].deleted);
    select(available.find(i=>i>=preferred)??available.at(-1)??-1);
  }
  function select(i){
    if(chapterSettings)chapterSettings.open=false;
    index=i;const part=desk.parts[i];draft={title:part?.title||'',beats:part?.beats||'',focus:part?.focus||'',remarks:part?.remarks||[]};dirty=false;
    try{const local=JSON.parse(localStorage.getItem(draftKey(i)));if(local){draft=local;dirty=true}}catch{}
    const lines=(draft.beats||"").split("\n");
    rows=(draft.remarks?.length?lines:lines.filter(v=>v.trim())).map((text,n)=>({text,remark:draft.remarks?.[n]||""}));
    if(!rows.length)rows=[{text:"",remark:""}];
    version=chapters.find(c=>c.desk_part===i+1)?.desk_version||part?.versions?.length||0;
    revisionBeat=null;comment='';manual=null;compare=false;openBeats=new Set([0]);
  }
  async function load(){
    busy=true;try{receive(await request('open'));selectAvailable(index);error=''}catch(e){error=e.message}finally{busy=false}
  }
  onMount(load);
  function edit(field,value){
    draft={...draft,[field]:value};dirty=true;
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
  function beginEdit(i){
    revisionBeat=null;
    manual={beat:i,line:candidate.lines[i],instruction:candidate.instructions?.[i]||''};
  }
  async function save(){
    if(!dirty)return;
    receive(await request('save',{draft}));dirty=false;
    try{localStorage.removeItem(draftKey(index))}catch{}
  }
  function deleteChapter(){
    const name=partName(part,index,chapters,$t('desk.untitled'));
    showConfirm($t(chapter?'desk.deleteLinkedConfirm':'desk.deleteConfirm',{name}),()=>action('delete'));
  }
  async function action(kind,mode='',target=index){
    if(incomplete&&kind!=='delete'&&kind!=='restore'){error=$t('desk.missingBeat');return}
    busy=true;error='';try{
      if(kind!=='restore')await save();
      if(kind==='save'){addToast($t('common.save'),'success');return}
      if(kind==='generate'){
        receive(await request(kind,{mode}));
        version=desk.parts[index].versions.length;
        revisionBeat=null;
        comment='';
      }else if(kind==='revise_beat'){
        receive(await request('generate',{beat:revisionBeat+1,comment:comment.trim()}));
        version=desk.parts[index].versions.length;
        revisionBeat=null;
        comment='';
      }else if(kind==='manual_outline'){
        const lines=[...candidate.lines];
        const instructions=lines.map((_,i)=>candidate.instructions?.[i]||'');
        lines[manual.beat]=manual.line;
        instructions[manual.beat]=manual.instruction;
        receive(await request(kind,{lines,instructions}));
        version=desk.parts[index].versions.length;
        manual=null;
      }else{
        receive(await request(kind,{part:target+1}));
        if(kind==='add')select(desk.parts.length-1);
        if(kind==='delete')selectAvailable(index);
        if(kind==='restore')select(target);
      }
    }catch(e){error=e.message}finally{busy=false}
  }
  async function decideSetting(action, suggestion, value) {
    if (busy || manual) return false;
    busy = true;
    error = '';
    try {
      if (action === 'apply_setting') {
        if (incomplete) {
          error = $t('desk.missingBeat');
          return false;
        }
        await save();
      }
      receive(await request(action, {suggestion, setting_value: value}));
      if (action !== 'dismiss_setting') {
        try { settings.set(await api('GET', '/api/settings')); }
        catch (e) { addToast(e.message, 'error'); }
      }
      return true;
    } catch (e) {
      error = e.message;
      addToast(e.message, 'error');
      return false;
    } finally { busy = false; }
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

      <div class="desk-workspace">
        <aside class="desk-chapter-list" aria-label={$t('desk.chapterList')}>
          <h3>{$t('desk.chapterList')} <span class="text-xs opacity-70">· {desk.parts.filter(item=>!item.deleted).length}</span></h3>
          {#each desk.parts as item,i}
            {#if !item.deleted}
            <button class="btn btn-sm desk-chapter" class:btn-primary={index===i} class:btn-ghost={index!==i}
              aria-current={index===i?'true':undefined} disabled={busy||!!manual} on:click={()=>select(i)}>
              <span>{i+1}.</span> <span>{partName(item,i,chapters,$t('desk.untitled'))}</span>
            </button>
            {/if}
          {/each}
          <button class="btn btn-outline btn-sm desk-add-part" disabled={busy||!!manual} on:click={()=>action('add')}>＋ {$t('desk.addChapter')}</button>
          {#if desk.parts.some(item=>item.deleted)}
            <details class="desk-deleted">
              <summary>{$t('desk.deletedChapters')}</summary>
              {#each desk.parts as item,i}
                {#if item.deleted}
                  <div>
                    <span>{i+1}. {partName(item,i,chapters,$t('desk.untitled'))}</span>
                    <button class="btn btn-ghost btn-xs" disabled={busy||!!manual} on:click={()=>action('restore','',i)}>{$t('desk.restoreChapter')}</button>
                  </div>
                {/if}
              {/each}
            </details>
          {/if}
        </aside>
        {#if part}
        <div class="desk-editor">
          <header class="desk-header">
            <div class="desk-toolbar">
              <h2 class="min-w-0 break-words font-semibold">{index+1}. {partName(part,index,chapters,$t('desk.untitled'))}</h2>
              <div class="desk-actions">
                <button class="btn btn-outline btn-sm" disabled={busy||!dirty||!!manual} on:click={()=>action('save')}>{$t('desk.save')}</button>
                <button class="btn btn-sm" class:btn-primary={!chapter?.outline||changed} class:btn-outline={chapter?.outline&&!changed} disabled={busy||!!manual||!draft.beats.trim()||incomplete} on:click={()=>action('generate',changed?'update':'rewrite')}>{$t(candidate?(changed?'flow.updateOutline':'flow.rewriteOutline'):'desk.generate')}</button>
                {#if chapter?.outline&&!manual}<a class="btn btn-sm" class:btn-primary={!changed} class:btn-ghost={changed} href="#writing" on:click={()=>focusWritingChapter(chapters)}>{$t(chapter.word_count?'desk.openChapter':'desk.write')}</a>{/if}
              </div>
            </div>
            <div class="desk-toolbar desk-toolbar-secondary">
              <div class="desk-actions">
                <details class="desk-menu" bind:this={chapterSettings}>
                  <summary class="btn btn-ghost btn-sm">{$t('desk.chapterSettings')}</summary>
                  <fieldset disabled={busy||!!manual} class="desk-menu-content space-y-3">
                    <label>{$t('desk.title')}<input class="input w-full" value={draft.title} placeholder={candidate?.title||$t('desk.untitled')} on:input={e=>edit('title',e.target.value)}></label>
                    <label>{$t('desk.chapterRemarks')}<textarea rows="3" class="textarea w-full" value={draft.focus} aria-describedby="desk-chapter-remarks-help" on:input={e=>edit('focus',e.target.value)}></textarea></label>
                    <p id="desk-chapter-remarks-help" class="text-xs opacity-75">{$t('desk.chapterRemarksHelp')}</p>
                    <button class="btn btn-outline btn-error btn-sm" on:click={deleteChapter}>{$t('desk.deleteChapter')}</button>
                  </fieldset>
                </details>
                {#if candidate}
                  <select class="select select-sm desk-version" aria-label={$t('desk.version')} bind:value={version} disabled={busy||!!manual} on:change={()=>{revisionBeat=null;comment=''}}>
                    {#each part.versions as _,i}<option value={i+1}>{$t('desk.version')} {i+1}{chapter?.desk_version===i+1?' ✓':''}</option>{/each}
                  </select>
                  {#if chapter?.desk_version===version}<small class="desk-active">{$t('desk.active')}</small>
                  {:else}<button class="btn btn-outline btn-sm" disabled={busy||!!manual} on:click={()=>action('use')}>{$t('desk.use')}</button>{/if}
                {/if}
              </div>
              <div class="desk-actions">
                {#if candidate&&!manual}
                  {#if adopted&&adopted!==candidate}<button class="btn btn-ghost btn-sm" aria-expanded={compare} on:click={()=>compare=!compare}>{$t('flow.compare')}</button>{/if}
                {/if}
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
            {#if draft.focus?.trim()}<p class="desk-chapter-guidance"><strong>{$t('desk.chapterRemarks')}</strong> {draft.focus}</p>{/if}
          </header>

          {#key index + ":" + version}
            <DeskSettingSuggestions suggestions={candidate?.setting_suggestions || []}
              decisions={settingDecisions} part={index+1} {version}
              adopted={!!candidate&&candidate===adopted} busy={busy||!!manual} decide={decideSetting}/>
          {/key}

          {#if compare&&adopted&&candidate!==adopted}
            <div class="candidate-diff">
              {#each [adopted,candidate] as entry}<div>{#each entry.lines as line,i}<p>{line}</p>{#if entry.instructions?.[i]}<p class="mb-2 font-medium">{entry.instructions[i]}</p>{/if}{/each}</div>{/each}
            </div>
          {/if}

          <fieldset class="desk-beat-cards min-w-0" disabled={busy}>
            <legend class="sr-only">{$t('desk.beat')}</legend>
            {#each Array(Math.max(rows.length,candidate?.lines?.length||0)) as _,i}
              <section class="desk-beat">
                <button class="desk-beat-toggle" disabled={manual?.beat===i} aria-expanded={openBeats.has(i)} aria-controls={'desk-beat-'+i} on:click={()=>toggleBeat(i)}>
                  <strong>{$t('desk.beat')} {i+1}</strong>
                  <span>{rows[i]?.text||candidate?.lines?.[i]||''}</span>
                  <span aria-hidden="true">{openBeats.has(i)?'▾':'▸'}</span>
                  {#if !openBeats.has(i)&&(rows[i]?.remark||candidate?.instructions?.[i])}<small class="desk-beat-note">{rows[i]?.remark||candidate.instructions[i]}</small>{/if}
                </button>
                {#if openBeats.has(i)}
                  <div id={'desk-beat-'+i} class="desk-beat-fields">
                    <div class="desk-inputs">
                      {#if rows[i]}
                        <label>{$t('desk.beatDraft')}<textarea class="textarea w-full" rows="3" value={rows[i].text} on:input={e=>editRow(i,'text',e.target.value)}></textarea></label>
                        <label>{$t('desk.beatRemarks')}<textarea class="textarea w-full" rows="2" value={rows[i].remark} on:input={e=>editRow(i,'remark',e.target.value)}></textarea></label>
                        <button class="btn btn-ghost btn-xs" disabled={!!manual} on:click={()=>removeBeat(i)}>{$t('desk.removeBeat')}</button>
                      {/if}
                    </div>
                    {#if manual?.beat===i}
                      <div class="desk-outputs">
                        <label>{$t('desk.beatOutline')}<textarea class="textarea w-full" rows="4" bind:value={manual.line}></textarea></label>
                        <label>{$t('desk.instructions')}<textarea class="textarea w-full" rows="2" bind:value={manual.instruction}></textarea></label>
                        <div class="desk-actions">
                          <button class="btn btn-primary btn-sm" disabled={!manual.line.trim()} on:click={()=>action('manual_outline')}>{$t('common.save')}</button>
                          <button class="btn btn-ghost btn-sm" on:click={()=>manual=null}>{$t('common.cancel')}</button>
                        </div>
                      </div>
                    {:else if candidate?.lines?.[i]}
                      <div class="desk-outputs">
                        <h4>{$t('desk.beatOutline')}</h4><p class="whitespace-pre-wrap break-words">{candidate.lines[i]}</p>
                        <h4>{$t('desk.instructions')}</h4><p class="whitespace-pre-wrap break-words">{candidate.instructions?.[i]||'—'}</p>
                        <div class="desk-actions">
                          <button class="btn btn-ghost btn-xs" disabled={!!manual} on:click={()=>beginEdit(i)}>{$t('desk.editOutputs')}</button>
                          <button class="btn btn-ghost btn-xs" disabled={!!manual} aria-expanded={revisionBeat===i}
                            on:click={()=>revisionBeat===i?revisionBeat=null:openRevision(i)}>{$t('desk.reviseOutline')}</button>
                        </div>
                        {#if revisionBeat===i}
                          <label>{$t('desk.comment')}
                            <textarea class="textarea w-full" rows="2" bind:value={comment} bind:this={revisionInput}></textarea>
                          </label>
                          <div class="desk-actions">
                            <button class="btn btn-outline btn-sm" disabled={!comment.trim()||incomplete} on:click={()=>action('revise_beat')}>{$t('desk.revise')}</button>
                            <button class="btn btn-ghost btn-sm" on:click={()=>revisionBeat=null}>{$t('common.cancel')}</button>
                          </div>
                        {/if}
                      </div>
                    {/if}
                  </div>
                {/if}
              </section>
            {/each}
            <button class="btn btn-outline btn-sm justify-self-start" disabled={!!manual} on:click={addBeat}>＋ {$t('desk.addBeat')}</button>
          </fieldset>
        </div>
        {:else}
          <p class="desk-empty">{$t('desk.noChapters')}</p>
        {/if}
      </div>
    {:else if busy}
      <p role="status">{$t('desk.working')}</p>
    {/if}
  {/if}
</section>
