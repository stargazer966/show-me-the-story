<script>
  import { onMount } from 'svelte';
  import { api, apiFetch } from '../lib/api.js';
  import { config, skills, addToast, taskRunning, logEntries, confirmModal } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';
  let mode='paste', markdown='', selectedFile=null, folderFiles=[], overwrite=false, installing=false, detail=null, wasRunning=false, staticCheck=null;
  async function load(){try{skills.set(await api('GET','/api/skill-library'));}catch(e){addToast(e.message,'error')}}
  const optimizationMessages = {
    'log.skill_optimization_done': 'alert-success',
    'log.skill_optimization_failed': 'alert-error',
    'log.skill_optimization_needs_review': 'alert-warning',
  };
  $: optimizationResult = $logEntries.findLast(entry => Object.hasOwn(optimizationMessages, entry.msg_key));
  onMount(load); $: if(wasRunning&&!$taskRunning)load(); $: wasRunning=$taskRunning;
  async function toggleSkill(id,enabled){try{await api('PUT','/api/skills/'+encodeURIComponent(id)+'/toggle',{enabled});addToast(enabled?$t('skills.toast.enabled'):$t('skills.toast.disabled'),'success');config.set(await api("GET","/api/config"));await load()}catch(e){addToast(e.message,'error');await load()}}
  let name='', entry='', scopes=['chapter.generate','chapter.revise'];
  const choices=['chapter.generate','chapter.revise','outline.generate','outline.revise','chapter.polish','assistant.chat'];
  async function submit(preview) {
    installing=true;
    try {
      const query=new URLSearchParams({name,entry,validate_only:String(preview)});
      for(const scope of scopes) query.append('scope',scope);
      const url='/api/skill-library/install?'+query;
      let result;
      if(mode==='paste') {
        if(!markdown.trim()) throw new Error($t('skills.install.needContent'));
        result=await api('POST',url,{markdown,overwrite});
      } else {
        const files=mode==='folder'?folderFiles:[selectedFile].filter(Boolean);
        if(!files.length) throw new Error($t('skills.install.needFile'));
        const data=new FormData();
        data.append('overwrite',String(overwrite));
        if(mode==='zip') data.append('zip',files[0]);
        else {
          data.append('paths',JSON.stringify(files.map(f=>f.webkitRelativePath||f.name)));
          files.forEach(f=>data.append('files',f));
        }
        result=await (await apiFetch(url,{method:'POST',body:data})).json();
      }
      if(preview){staticCheck=result;return}
      addToast($t('skills.install.done'),'success'); markdown='';selectedFile=null;folderFiles=[];name='';entry='';staticCheck=null;overwrite=false;await load();
    } catch(e){staticCheck={error:e.message};addToast(e.message,'error')} finally{installing=false}
  }
  $: {mode; markdown; selectedFile; folderFiles; name; scopes; entry; staticCheck=null;}
  async function validateSkill(id){try{await api('POST',`/api/skill-library/${encodeURIComponent(id)}/validate`);addToast($t('skills.validate.started'),'info');await load()}catch(e){addToast(e.message,'error')}}
  async function optimizeSkill(id){try{await api('POST',`/api/skill-library/${encodeURIComponent(id)}/optimize`);addToast($t('skills.optimize.started'),'info')}catch(e){addToast(e.message,'error')}}
  let scopeEditor = null, editedScopes = [], savingScopes = false, scopeError = '';
  async function editScopes(id) {
    try {
      const item = await api('GET', '/api/skill-library/' + encodeURIComponent(id));
      editedScopes = [...item.effective_scopes];
      scopeError = '';
      scopeEditor = item;
    } catch (e) { addToast(e.message, 'error'); }
  }
  async function saveScopes(reset = false) {
    savingScopes = true;
    scopeError = '';
    try {
      const result = await api('PUT', '/api/skill-library/' + encodeURIComponent(scopeEditor.skill.id) + '/scopes',
        { scopes: reset ? [] : editedScopes, reset });
      skills.set(result.skills);
      config.set(result.config);
      scopeEditor = null;
      addToast($t('skills.scopes.saved'), 'success');
    } catch (e) { scopeError = e.message; }
    finally { savingScopes = false; }
  }
  async function showDetail(id){try{detail=await api('GET',`/api/skill-library/${encodeURIComponent(id)}`)}catch(e){addToast(e.message,'error')}}
  function removeSkill(id){confirmModal.set({message:$t('skills.delete.confirm'),onConfirm:async()=>{try{await api('DELETE',`/api/skill-library/${encodeURIComponent(id)}`);addToast($t('skills.delete.done'),'success');detail=null;await load()}catch(e){addToast(e.message,'error')}}})}
  const blocked=s=>['failed','needs_optimization','validating'].includes(s);
  function statusIcon(s){return s==='passed'?'':s==='failed'?'':s==='needs_optimization'?'':s==='validating'?'…':'!'}
  function statusClass(s){return s==='passed'?'badge-success':s==='failed'?'badge-error':s==='needs_optimization'?'badge-warning':s==='validating'?'badge-info':'badge-ghost'}
</script>

<div class="space-y-4">
 <div class="card bg-base-200"><div role="region" aria-label={$t("skills.install.title")} class="card-body p-4" on:dragover|preventDefault on:drop|preventDefault={e=>{const f=e.dataTransfer.files[0];if(f&&!installing){entry='';mode=f.name.toLowerCase().endsWith('.zip')?'zip':'markdown';selectedFile=f}}}>
  <h2 class="card-title text-base">{$t('skills.install.title')}</h2><div class="tabs tabs-box w-fit">{#each ['paste','markdown','zip','folder'] as item}<button class:tab-active={mode===item} class="tab" on:click={()=>{mode=item;entry='';selectedFile=null}}>{$t(`skills.install.${item}`)}</button>{/each}</div>
  {#if mode==='paste'}<textarea class="textarea textarea-bordered w-full h-40 font-mono text-xs" bind:value={markdown} disabled={$taskRunning} placeholder={$t("wf.paste")}></textarea>
  {:else if mode==='folder'}<input class="file-input file-input-bordered w-full" type="file" webkitdirectory multiple disabled={$taskRunning} on:change={e=>folderFiles=Array.from(e.currentTarget.files)}/>{#if folderFiles.length}<p class="text-xs opacity-60">{folderFiles.length} {$t('skills.install.files')}</p>{/if}
  {:else}<input class="file-input file-input-bordered w-full" type="file" accept={mode==='zip'?'.zip':'.md,.txt,text/plain,text/markdown'} disabled={$taskRunning} on:change={e=>selectedFile=e.currentTarget.files[0]}/>{/if}
  {#if selectedFile}<small>{selectedFile.name}</small>{/if}
  <label class="label cursor-pointer justify-start gap-3"><input type="checkbox" class="checkbox checkbox-sm" bind:checked={overwrite} disabled={$taskRunning}/><span>{$t('skills.install.overwrite')}</span></label>
  <input class="input" bind:value={name} placeholder={$t('wf.skillName')} aria-label={$t('wf.skillName')}/>
  <fieldset><legend>{$t('wf.scopes')}</legend><div class="flex gap-3 flex-wrap">{#each choices as scope}<label><input class="checkbox checkbox-xs" type="checkbox" bind:group={scopes} value={scope}/> {scope}</label>{/each}</div></fieldset>
  {#if staticCheck?.entries}<label>{$t('wf.entry')} <select class="select" on:change={e=>entry=e.currentTarget.value}><option value="">—</option>{#each staticCheck.entries as item}<option>{item}</option>{/each}</select></label>{/if}
  <button class="btn btn-outline btn-sm" disabled={$taskRunning||installing} on:click={()=>submit(true)}>{$t('wf.preview')}</button>
  {#if staticCheck?.valid}<strong>{staticCheck.manifest.name}</strong><small>{staticCheck.manifest.applies_to.join(', ')}</small><pre class="whitespace-pre-wrap max-h-48 overflow-auto">{staticCheck.preview}</pre><button class="btn btn-primary btn-sm" disabled={$taskRunning||installing} on:click={()=>submit(false)}>{$t('skills.install.button')}</button>{/if}

 </div></div>
 <div class="card bg-base-200"><div class="card-body p-4"><h2 class="card-title text-base">{$t('skills.title')}</h2><p class="text-sm opacity-60 mb-3">{$t('skills.intro')}</p>{#if optimizationResult}
   <div class="alert alert-sm {optimizationMessages[optimizationResult.msg_key]}" role="status">{optimizationResult.msg}</div>
  {/if}
  <div class="overflow-x-auto"><table class="table table-sm">
  <thead><tr><th>{$t('skills.col.name')}</th><th>{$t('skills.col.scope')}</th><th>{$t('skills.col.validation')}</th><th>{$t('skills.col.source')}</th><th>{$t('skills.col.actions')}</th><th>{$t('skills.col.enabled')}</th></tr></thead><tbody>
  {#if $skills.length===0}<tr><td colspan="6" class="text-center opacity-50 py-8">{$t('skills.empty')}</td></tr>{/if}
  {#each $skills as sv}<tr>
   <td><button class="link link-hover font-medium text-left" on:click={()=>showDetail(sv.skill.id)}>{sv.skill.name}</button><div class="text-xs opacity-50 max-w-xs">{sv.skill.description}</div></td>
   <td><div class="flex flex-wrap gap-1">{#each sv.effective_scopes||sv.skill.applies_to||[] as scope}<span class="badge badge-outline badge-xs">{scope}</span>{/each}</div></td>
   <td><span class="badge badge-sm {statusClass(sv.validation_status)}" title={$t(`skills.status.${sv.validation_status}`)}>{statusIcon(sv.validation_status)}</span></td>
   <td>{sv.skill.source==='builtin'?$t('skills.source.builtin'):$t('skills.source.user')}</td>
   <td><div class="flex flex-wrap gap-1"><button class="btn btn-outline btn-xs" disabled={$taskRunning} on:click={() => editScopes(sv.skill.id)}>{$t('skills.scopes.edit')}</button>{#if sv.can_validate}<button class="btn btn-outline btn-xs" disabled={$taskRunning} on:click={()=>validateSkill(sv.skill.id)}>{$t('skills.validate.button')}</button>{/if}{#if sv.validation_status==='failed'||sv.validation_status==='needs_optimization'}<button class="btn btn-warning btn-xs" disabled={$taskRunning} on:click={()=>optimizeSkill(sv.skill.id)}>{$t('skills.optimize.button')}</button>{/if}{#if sv.can_delete}<button class="btn btn-error btn-outline btn-xs" disabled={$taskRunning} on:click={()=>removeSkill(sv.skill.id)}>{$t('common.delete')}</button>{/if}</div></td>
   <td><input type="checkbox" class="toggle toggle-primary toggle-sm" checked={sv.enabled} disabled={$taskRunning||blocked(sv.validation_status)} on:change={e=>toggleSkill(sv.skill.id,e.target.checked)}/></td>
  </tr>{/each}</tbody></table></div></div></div>
</div>

{#if detail}<div class="modal modal-open"><div class="modal-box max-w-3xl"><h3 class="font-bold text-lg">{detail.skill.name}</h3><p class="text-sm opacity-70">{detail.skill.description}</p><div class="flex flex-wrap gap-1 my-3">{#each detail.effective_scopes||detail.skill.applies_to||[] as scope}<span class="badge badge-outline badge-sm">{scope}</span>{/each}</div>{#if detail.skill.validation}<div class="alert mb-3"><div><strong>{$t(`skills.status.${detail.validation_status}`)}</strong><p>{detail.skill.validation.summary}</p>{#if detail.skill.validation.issues?.length}<ul class="list-disc ml-5 text-sm">{#each detail.skill.validation.issues as issue}<li>{issue}</li>{/each}</ul>{/if}</div></div>{/if}{#if detail.skill.resources?.length}<p class="text-xs mb-2">{$t('skills.resources')}: {detail.skill.resources.join(', ')}</p>{/if}<pre class="bg-base-300 rounded p-3 max-h-96 overflow-auto whitespace-pre-wrap text-xs">{detail.skill.content}</pre><div class="modal-action"><button class="btn" on:click={()=>detail=null}>{$t('common.close')}</button></div></div><button class="modal-backdrop" on:click={()=>detail=null}>close</button></div>{/if}

{#if scopeEditor}
  <div class="modal modal-open" role="dialog" aria-modal="true" aria-labelledby="skill-scope-title">
    <div class="modal-box max-w-xl space-y-4">
      <div>
        <h3 id="skill-scope-title" class="text-lg font-semibold">{$t('skills.scopes.edit')} · {scopeEditor.skill.name}</h3>
        <p class="text-sm opacity-75">{$t('skills.scopes.project')}</p>
      </div>
      <fieldset class="grid grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2" disabled={savingScopes || $taskRunning}>
        <legend class="sr-only">{$t('skills.col.scope')}</legend>
        {#each scopeEditor.available_scopes as scope}
          <div class="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2">
            <input id={'edit-scope-' + scope} class="checkbox checkbox-sm" type="checkbox"
              aria-describedby={'scope-help-' + scope} bind:group={editedScopes} value={scope} />
            <div class="min-w-0">
              <label for={'edit-scope-' + scope} class="block break-words font-medium">{scope}</label>
              <p id={'scope-help-' + scope} class="mt-0.5 text-xs leading-snug text-base-content/75">{$t('skills.scopeHelp.' + scope)}</p>
            </div>
          </div>
        {/each}
      </fieldset>
      {#if scopeError}<p class="text-error text-sm" role="alert">{scopeError}</p>{/if}
      <div class="modal-action flex-wrap">
        <button class="btn btn-ghost btn-sm mr-auto" disabled={savingScopes || $taskRunning} on:click={() => saveScopes(true)}>{$t('skills.scopes.reset')}</button>
        <button class="btn btn-ghost btn-sm" disabled={savingScopes} on:click={() => scopeEditor = null}>{$t('common.cancel')}</button>
        <button class="btn btn-primary btn-sm" disabled={savingScopes || $taskRunning || !editedScopes.length} on:click={() => saveScopes()}>{$t('common.save')}</button>
      </div>
    </div>
  </div>
{/if}
