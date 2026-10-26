'use strict';
const $ = id => document.getElementById(id);
let data, session = [], reviewed = 0, sessionSize = 0, visible = 30;
const formatDate = value => new Date(value).toLocaleDateString(undefined, {month:'short', day:'numeric'});
async function request(path, options = {}) {
  const response = await fetch(path, { ...options, headers: {'X-Remind-Request':'1', ...options.headers} });
  if (!response.ok) throw new Error((await response.text()).trim() || 'Request failed.');
  if (response.headers.get('content-type')?.includes('application/json')) return response.json();
  return response.text();
}
function notice(message) { $('notice').textContent = message; $('notice').hidden = false; }
function card(item) {
  const node = $('highlight-template').content.cloneNode(true);
  node.querySelector('.book-title').textContent = item.Title;
  node.querySelector('blockquote').textContent = item.Highlight;
  node.querySelector('.author').textContent = item.Author;
  const details = [];
  if (item.Page) details.push(`Page ${item.Page}`);
  if (item.Location) details.push(`Location ${item.Location}`);
  if (item.Review?.SendCount) details.push(`Revisited ${item.Review.SendCount} ${item.Review.SendCount === 1 ? 'time' : 'times'}`);
  node.querySelector('.metadata').textContent = details.join(' · ');
  node.querySelector('.review-status').textContent = item.Due !== false ? 'Ready to revisit' : `Returns ${formatDate(item.Review.NextDueAt)}`;
  return node;
}
function empty(title, description, importButton = false) {
  const node = document.createElement('div'); node.className = 'empty';
  const icon = document.createElement('div'); icon.className = 'empty-icon'; icon.textContent = '❧';
  const h = document.createElement('h3'); h.textContent = title;
  const p = document.createElement('p'); p.textContent = description;
  node.append(icon, h, p);
  if (importButton) { const b = document.createElement('button'); b.className = 'button primary'; b.textContent = 'Import your first highlights'; b.onclick = openImport; node.append(b); }
  return node;
}
function renderReview() {
  const area = $('review-area'); area.replaceChildren();
  $('session-count').textContent = sessionSize ? `${Math.min(reviewed, sessionSize)} of ${sessionSize} revisited` : '';
  if (!data.highlights.length) { area.append(empty('Your next favorite thought is already in a book.', 'Bring in your Kindle highlights to start a small daily habit.', true)); return; }
  if (!session.length) {
    if (data.due > 0) {
      area.append(empty('A few good words, well remembered.', 'Your session is complete. Come back tomorrow, or spend another moment with your books.'));
      const b = document.createElement('button'); b.className = 'button secondary'; b.textContent = 'Review a few more'; b.onclick = () => { session = data.session || []; reviewed = 0; sessionSize = session.length; renderReview(); }; area.firstChild.append(b);
    } else area.append(empty('All caught up.', 'Your highlights are resting. They will return when it is time to revisit them.'));
    return;
  }
  const current = session[0]; area.append(card(current));
  const actions = document.createElement('div'); actions.className = 'review-actions';
  const hint = document.createElement('span'); hint.className = 'progress'; hint.textContent = `Highlight ${reviewed + 1} of ${sessionSize} · Take your time.`;
  const buttons = document.createElement('div'); buttons.className = 'action-buttons';
  for (const [action, label, style] of [['again', 'See tomorrow', 'secondary'], ['remembered', 'Remembered ✓', 'primary']]) {
    const button = document.createElement('button'); button.className = `button ${style}`; button.textContent = label;
    button.onclick = async () => {
      buttons.querySelectorAll('button').forEach(b => b.disabled = true);
      try {
        await request('/api/review', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({id:current.ID, action})});
        session.shift(); reviewed++;
        await load(false);
      } catch(error) { notice(error.message); buttons.querySelectorAll('button').forEach(b => b.disabled = false); }
    };
    buttons.append(button);
  }
  actions.append(hint,buttons); area.querySelector('article').append(actions);
}
function renderLibrary() {
  const query = $('search').value.trim().toLowerCase(), book = $('book-filter').value;
  const results = data.highlights.filter(h => (!book || h.Title === book) && (!$('due-filter').checked || h.Due) && (!query || `${h.Highlight} ${h.Title} ${h.Author}`.toLowerCase().includes(query)));
  $('results-count').textContent = `${results.length} ${results.length === 1 ? 'highlight' : 'highlights'}`;
  $('library-grid').replaceChildren(...results.slice(0,visible).map(card));
  if (!results.length) $('library-grid').append(empty('No highlights here yet.', query || book ? 'Try a different search or book.' : 'Import your clippings to fill your library.', !data.highlights.length));
  $('show-more').hidden = results.length <= visible;
}
async function load(resetSession = true) {
  data = await request('/api/library');
  $('stat-highlights').textContent = data.highlights.length.toLocaleString(); $('stat-books').textContent = data.books; $('stat-due').textContent = data.due; $('review-badge').textContent = data.due;
  if (resetSession) { session = data.session || []; reviewed = 0; sessionSize = session.length; }
  const selected = $('book-filter').value;
  $('book-filter').replaceChildren(new Option('All books',''), ...[...new Set(data.highlights.map(h => h.Title))].sort().map(title => new Option(title,title)));
  $('book-filter').value = selected;
  $('email-status').textContent = data.emailTime ? 'Daily delivery enabled' : data.emailConfigured ? 'Delivery configured' : 'Preview mode';
  $('email-description').textContent = data.emailTime ? `${data.dailyCount} due highlights, delivered at ${data.emailTime} (${data.timezone}).` : `Preview a digest of ${data.dailyCount} due highlights. Automatic delivery is not enabled.`;
  const delivery = data.delivery || {};
  const deliveryMessages = {
    accepted: 'Your latest digest was accepted by the email provider.',
    failed: 'Your latest digest attempt failed. Check the server logs before trying again.',
    in_progress: 'Your latest attempt has no confirmed outcome yet.',
    accepted_state_error: 'Your digest was accepted, but its review schedule could not be saved. Check the server logs.'
  };
  $('email-delivery').textContent = deliveryMessages[delivery.outcome] || 'No email attempts recorded yet.';
  if (delivery.last_success_at) $('email-delivery').textContent += ` Last accepted: ${new Date(delivery.last_success_at).toLocaleString()}.`;
  renderReview(); renderLibrary();
}
function openImport() { $('import-error').hidden = true; $('import-dialog').showModal(); }
$('date').textContent = new Date().toLocaleDateString(undefined, {weekday:'long', month:'long', day:'numeric'}).toUpperCase();
$('import-top').onclick = openImport;
$('close-import').onclick = () => $('import-dialog').close();
for (const view of ['today','library','email']) $('tab-' + view).onclick = () => {
  for (const other of ['today','library','email']) { $(other + '-view').hidden = other !== view; $('tab-' + other).classList.toggle('active', other === view); $('tab-' + other).setAttribute('aria-selected', String(other === view)); }
};
for (const id of ['search','book-filter','due-filter']) $(id).addEventListener('input', () => { visible = 30; if(data) renderLibrary(); });
$('show-more').onclick = () => { visible += 30; renderLibrary(); };
$('load-preview').onclick = () => { $('email-preview').hidden = false; $('email-preview').src = '/api/email-preview'; };
$('import-form').onsubmit = async event => {
  event.preventDefault(); const file = $('file').files[0];
  if (!file || file.size > 10 * 1024 * 1024) { $('import-error').textContent = 'Choose a file smaller than 10 MB.'; $('import-error').hidden = false; return; }
  $('import-submit').disabled = true; $('import-submit').textContent = 'Importing…';
  try {
    const result = await request('/api/import', {method:'POST', body:new FormData($('import-form'))});
    $('import-dialog').close(); $('import-form').reset();
    notice(`Added ${result.added} new highlights. ${result.duplicates} already in your library.`);
    await load();
  } catch(error) { $('import-error').textContent = error.message; $('import-error').hidden = false; }
  finally { $('import-submit').disabled = false; $('import-submit').textContent = 'Import highlights'; }
};
load().catch(error => { $('review-area').replaceChildren(empty('Could not open your library.', error.message)); notice('Refresh the page to try again.'); });
