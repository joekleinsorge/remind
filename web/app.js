'use strict';
const $ = id => document.getElementById(id);
let data, session = [], reviewed = 0, skipped = 0, sessionSize = 0, visible = 30, lastAction = null, busy = false;
const formatDate = value => {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date.toLocaleDateString(undefined, {month:'short', day:'numeric'});
};
async function request(path, options = {}) {
  const response = await fetch(path, { ...options, headers: {'X-Remind-Request':'1', ...options.headers} });
  if (!response.ok) throw new Error((await response.text()).trim() || 'Request failed.');
  if (response.headers.get('content-type')?.includes('application/json')) return response.json();
  return response.text();
}
const post = (path, body) => request(path, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)});
function notice(message) { $('notice').textContent = message; $('notice').hidden = false; }
function button(label, handler, style = 'secondary') {
  const b = document.createElement('button'); b.type = 'button'; b.className = `button ${style}`; b.textContent = label; b.onclick = handler; b.disabled = busy; return b;
}
function renderUndo() {
  $('undo-bar').hidden = !lastAction;
  $('undo-description').textContent = lastAction?.action === 'skip' ? 'Skipped for this session. Schedule unchanged.' : 'Review saved.';
  $('undo-review').disabled = busy;
}
async function curate(id, changes) {
  if (busy) return;
  busy = true;
  try { await post('/api/curate', {id, ...changes}); await load(false); }
  catch (error) { notice(error.message); }
  finally { busy = false; renderReview(); renderLibrary(); renderUndo(); }
}
function card(item) {
  const node = $('highlight-template').content.cloneNode(true), review = item.Review || {};
  node.querySelector('.book-title').textContent = item.Title;
  node.querySelector('blockquote').textContent = item.Highlight;
  node.querySelector('.author').textContent = item.Author;
  const details = [];
  if (item.Page) details.push(`Page ${item.Page}`);
  if (item.Location) details.push(`Location ${item.Location}`);
  if (review.send_count) details.push(`Revisited ${review.send_count} ${review.send_count === 1 ? 'time' : 'times'}`);
  if (review.email_count) details.push(`Emailed ${review.email_count} ${review.email_count === 1 ? 'time' : 'times'}`);
  node.querySelector('.metadata').textContent = details.join(' · ');
  const due = formatDate(review.next_due_at);
  node.querySelector('.review-status').textContent = review.archived ? 'Archived' : item.Due ? 'Ready to revisit' : due ? `Returns ${due}` : 'Delivery pending';
  const tools = document.createElement('div'); tools.className = 'highlight-tools';
  const favorite = button(review.favorite ? '★ Favorited' : '☆ Favorite', () => curate(item.ID, {favorite:!review.favorite}));
  favorite.setAttribute('aria-pressed', String(!!review.favorite));
  tools.append(favorite, button(review.archived ? 'Unarchive' : 'Archive', () => curate(item.ID, {archived:!review.archived})));
  const notes = document.createElement('details'); notes.className = 'notes';
  const summary = document.createElement('summary'); summary.textContent = review.note ? 'Your note' : 'Add a note';
  const label = document.createElement('label'); label.textContent = 'What does this passage mean to you?';
  const textarea = document.createElement('textarea'); textarea.value = review.note || ''; textarea.rows = 3; textarea.maxLength = 10000; textarea.disabled = busy;
  label.append(textarea);
  const save = button('Save note', () => curate(item.ID, {note:textarea.value}));
  const hint = document.createElement('span'); hint.className = 'muted'; hint.textContent = 'Notes save when you choose Save note.';
  notes.append(summary, label, save, hint);
  node.querySelector('article').append(tools, notes);
  return node;
}
function empty(title, description, importButton = false) {
  const node = document.createElement('div'); node.className = 'empty';
  const h = document.createElement('h3'); h.textContent = title;
  const p = document.createElement('p'); p.textContent = description;
  node.append(h, p);
  if (importButton) node.append(button('Import your first highlights', openImport, 'primary'));
  return node;
}
function startSession() {
  session = (data.session || []).map(h => h.ID); reviewed = 0; skipped = 0; sessionSize = session.length; lastAction = null;
  renderReview(); renderUndo();
}
function renderReview() {
  const area = $('review-area'); area.replaceChildren();
  $('session-count').textContent = sessionSize ? `${reviewed + skipped} of ${sessionSize} passages` : '';
  if (!data.highlights.length) { area.append(empty('Your next favorite thought is already in a book.', 'Bring in your Kindle highlights to start a small daily habit.', true)); return; }
  if (!session.length) {
    const title = sessionSize ? `You revisited ${reviewed} ${reviewed === 1 ? 'passage' : 'passages'}.` : 'More good words will return.';
    const description = `${skipped ? `${skipped} skipped for now. ` : ''}${data.due ? 'Take a pause, or explore a few more passages.' : 'You’re caught up. Browse your library anytime.'}`;
    const done = empty(title, description);
    if (data.due > 0) done.append(button('Explore a few more', startSession));
    done.append(button('Browse your library', () => showView('library'))); area.append(done); return;
  }
  const current = data.highlights.find(h => h.ID === session[0]);
  area.append(card(current));
  const actions = document.createElement('div'); actions.className = 'review-actions';
  const hint = document.createElement('span'); hint.className = 'progress'; hint.textContent = `Passage ${reviewed + skipped + 1} of ${sessionSize}`;
  const buttons = document.createElement('div'); buttons.className = 'action-buttons';
  for (const [action, label, style] of [['skip', 'Skip', 'secondary'], ['again', 'Show again soon', 'secondary'], ['remembered', 'Next highlight', 'primary']]) {
    const b = button(label, async () => {
      if (busy) return;
      busy = true; renderReview(); renderUndo();
      const previous = {session:[...session], reviewed, skipped};
      try {
        const result = action === 'skip' ? {} : await post('/api/review', {id:current.ID, action});
        lastAction = {id:current.ID, action, token:result.undoToken, previous};
        session.shift();
        if (action === 'skip') { skipped++; } else reviewed++;
        // The mutation succeeded; keep Undo even if refreshing the library fails.
        if (action !== 'skip') await load(false);
      } catch(error) { notice(error.message); }
      finally { busy = false; renderReview(); renderLibrary(); renderUndo(); }
    }, style);
    b.title = action === 'skip' ? 'Move on without changing this passage’s schedule.' : action === 'again' ? 'Bring this passage back tomorrow.' : 'Mark revisited and bring this passage back later.';
    buttons.append(b);
  }
  actions.append(hint,buttons); area.querySelector('article').append(actions);
}
function renderLibrary() {
  const query = $('search').value.trim().toLowerCase(), book = $('book-filter').value;
  const results = data.highlights.filter(h => (!$('favorite-filter').checked || h.Review.favorite) && ($('archive-filter').checked || !h.Review.archived) && (!book || h.Title === book) && (!$('due-filter').checked || h.Due) && (!query || `${h.Highlight} ${h.Title} ${h.Author} ${h.Review.note || ''}`.toLowerCase().includes(query)));
  $('results-count').textContent = `${results.length} ${results.length === 1 ? 'highlight' : 'highlights'}`;
  $('library-grid').replaceChildren(...results.slice(0,visible).map(card));
  if (!results.length) $('library-grid').append(empty('No highlights match.', data.highlights.length ? 'Try clearing your search or changing the filters.' : 'Import your clippings to fill your library.', !data.highlights.length));
  $('show-more').hidden = results.length <= visible;
}
async function load(resetSession = true) {
  data = await request('/api/library');
  $('intro').classList.toggle('returning', data.highlights.length > 0);
  $('stat-highlights').textContent = data.highlights.length.toLocaleString(); $('stat-books').textContent = data.books; $('stat-due').textContent = data.due; $('review-badge').textContent = data.due;
  if (resetSession) { startSession(); }
  else {
    session = session.filter(id => data.highlights.some(h => h.ID === id && h.Due && !h.Review.archived));
    sessionSize = reviewed + skipped + session.length;
  }
  const selected = $('book-filter').value;
  $('book-filter').replaceChildren(new Option('All books',''), ...[...new Set(data.highlights.map(h => h.Title))].sort().map(title => new Option(title,title)));
  $('book-filter').value = selected;
  $('email-status').textContent = data.emailTime ? 'Daily delivery enabled' : data.emailConfigured ? 'Delivery configured' : 'Preview mode';
  $('email-description').textContent = data.emailTime ? `Up to ${data.dailyCount} passages delivered at ${data.emailTime} (${data.timezone}).` : `Preview up to ${data.dailyCount} passages. Automatic delivery is not enabled.`;
  const delivery = data.delivery || {};
  const messages = {
    accepted: 'Your latest digest was accepted by the email provider.',
    failed: 'Your latest digest attempt failed. Check the server logs.',
    in_progress: 'Your latest attempt has no confirmed outcome yet.',
    accepted_state_error: 'Your digest was accepted, but delivery history could not be saved. Check the server logs.'
  };
  $('email-delivery').textContent = messages[delivery.outcome] || 'No email attempts recorded yet.';
  if (delivery.last_success_at) $('email-delivery').textContent += ` Last accepted: ${new Date(delivery.last_success_at).toLocaleString()}.`;
  renderReview(); renderLibrary(); renderUndo();
}
function openImport() { $('import-error').hidden = true; $('import-dialog').showModal(); }
function showView(view) {
  for (const other of ['today','library','email']) { $(other + '-view').hidden = other !== view; $('tab-' + other).classList.toggle('active', other === view); $('tab-' + other).setAttribute('aria-selected', String(other === view)); }
}
$('date').textContent = new Date().toLocaleDateString(undefined, {weekday:'long', month:'long', day:'numeric'}).toUpperCase();
$('import-top').onclick = openImport;
$('close-import').onclick = () => $('import-dialog').close();
for (const view of ['today','library','email']) $('tab-' + view).onclick = () => showView(view);
for (const id of ['search','book-filter','due-filter','favorite-filter','archive-filter']) $(id).addEventListener('input', () => { visible = 30; if(data) renderLibrary(); });
$('show-more').onclick = () => { visible += 30; renderLibrary(); };
$('load-preview').onclick = () => { $('email-preview').hidden = false; $('email-preview').src = '/api/email-preview'; };
$('undo-review').onclick = async () => {
  if (!lastAction || busy) return;
  busy = true; renderUndo();
  const undo = lastAction;
  try {
    if (undo.action !== 'skip') await post('/api/undo', {id:undo.id, token:undo.token});
    session = undo.previous.session; reviewed = undo.previous.reviewed; skipped = undo.previous.skipped;
    lastAction = null;
    await load(false); notice('Undone. The previous schedule has been restored.');
  } catch(error) { notice(error.message); }
  finally { busy = false; renderReview(); renderLibrary(); renderUndo(); }
};
$('import-form').onsubmit = async event => {
  event.preventDefault(); const file = $('file').files[0];
  if (!file || file.size > 10 * 1024 * 1024) { $('import-error').textContent = 'Choose a file smaller than 10 MB.'; $('import-error').hidden = false; return; }
  $('import-submit').disabled = true; $('import-submit').textContent = 'Importing…';
  try {
    const result = await request('/api/import', {method:'POST', body:new FormData($('import-form'))});
    $('import-dialog').close(); $('import-form').reset();
    notice(`Added ${result.added} highlights; ${result.duplicates} duplicates. Skipped ${result.malformed} malformed records and ${result.unsupported} notes or bookmarks.`);
    await load();
  } catch(error) { $('import-error').textContent = error.message; $('import-error').hidden = false; }
  finally { $('import-submit').disabled = false; $('import-submit').textContent = 'Import highlights'; }
};
$('restore-top').onclick = () => { $('restore-error').hidden = true; $('restore-dialog').showModal(); };
$('close-restore').onclick = () => $('restore-dialog').close();
$('restore-form').onsubmit = async event => {
  event.preventDefault(); const file = $('backup-file').files[0];
  if (!file || file.size > 50 * 1024 * 1024) { $('restore-error').textContent = 'Choose a JSON backup smaller than 50 MB.'; $('restore-error').hidden = false; return; }
  $('restore-submit').disabled = true; $('restore-submit').textContent = 'Restoring…';
  try {
    const result = await request('/api/restore', {method:'POST', headers:{'Content-Type':'application/json'}, body:file});
    $('restore-dialog').close(); $('restore-form').reset();
    notice(`Restored ${result.added} highlights and ${result.reviews} review records. Existing data was kept.`); await load();
  } catch(error) { $('restore-error').textContent = error.message; $('restore-error').hidden = false; }
  finally { $('restore-submit').disabled = false; $('restore-submit').textContent = 'Merge backup into library'; }
};
load().catch(error => { $('review-area').replaceChildren(empty('Could not open your library.', error.message)); notice('Refresh the page to try again.'); });
