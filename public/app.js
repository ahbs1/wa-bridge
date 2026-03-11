// ===== Auth =====
const token = localStorage.getItem('wa_token');
if (!token) window.location.href = '/login';

function authHeaders() {
    return { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' };
}
function logout() {
    localStorage.removeItem('wa_token');
    localStorage.removeItem('wa_user');
    window.location.href = '/login';
}

// ===== Toast Notifications =====
function showToast(msg, type = 'info') {
    const c = document.getElementById('toastContainer');
    const t = document.createElement('div');
    t.className = `toast ${type}`;
    t.textContent = msg;
    c.appendChild(t);
    setTimeout(() => t.remove(), 3200);
}

// ===== State =====
let ws = null, sessions = {}, eventCount = 0, reconnectTimer = null;

// ===== DOM =====
const tabBtns = document.querySelectorAll('.tab');
const tabContents = document.querySelectorAll('.tab-content');
const sessionsGrid = document.getElementById('sessionsGrid');
const sessionCountEl = document.getElementById('sessionCount');
const createModal = document.getElementById('createModal');
const eventsList = document.getElementById('eventsList');
const eventCountEl = document.getElementById('eventCount');
const chatMessages = document.getElementById('chatMessages');
const chatSessionSelect = document.getElementById('chatSessionSelect');
const chatMsgType = document.getElementById('chatMsgType');
const chatPhone = document.getElementById('chatPhone');

const userEl = document.getElementById('currentUser');
if (userEl) userEl.textContent = localStorage.getItem('wa_user') || '';

// ===== Tabs =====
tabBtns.forEach(btn => {
    btn.addEventListener('click', () => {
        tabBtns.forEach(b => b.classList.remove('active'));
        tabContents.forEach(c => c.classList.remove('active'));
        btn.classList.add('active');
        document.getElementById(`tab-${btn.dataset.tab}`).classList.add('active');
    });
});

// ===== Message Type Toggle =====
const typeFields = { text: 'textFields', image: 'imageFields', document: 'documentFields', location: 'locationFields', poll: 'pollFields' };
chatMsgType.addEventListener('change', () => {
    Object.values(typeFields).forEach(id => document.getElementById(id).style.display = 'none');
    document.getElementById(typeFields[chatMsgType.value]).style.display = 'flex';
});

// ===== Phone Number Auto-format =====
chatPhone.addEventListener('input', updatePhoneHeader);
document.getElementById('chatCountryCode').addEventListener('input', function () {
    this.value = this.value.replace(/\D/g, '');
    updatePhoneHeader();
});

function updatePhoneHeader() {
    const cc = document.getElementById('chatCountryCode').value.replace(/\D/g, '');
    let v = chatPhone.value.replace(/\D/g, '');
    if (v.startsWith('0')) v = v.substring(1);
    chatPhone.value = v;
    const name = document.getElementById('chatContactName');
    const status = document.getElementById('chatContactStatus');
    if (v.length > 0) {
        name.textContent = '+' + cc + v;
        status.textContent = 'WhatsApp Contact';
    } else {
        name.textContent = 'Select a contact';
        status.textContent = 'Enter a phone number below';
    }
}

function getFullPhone() {
    const cc = document.getElementById('chatCountryCode').value.replace(/\D/g, '') || '62';
    let v = chatPhone.value.replace(/\D/g, '');
    if (v.startsWith('0')) v = v.substring(1);
    return cc + v;
}

// ===== WebSocket =====
function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${protocol}//${window.location.host}/ws`);
    ws.onopen = () => { if (reconnectTimer) clearTimeout(reconnectTimer); };
    ws.onclose = () => { reconnectTimer = setTimeout(connect, 3000); };
    ws.onmessage = (e) => { try { handleWS(JSON.parse(e.data)); } catch (err) { console.error('[WS]', err); } };
}

function handleWS(data) {
    switch (data.type) {
        case 'sessions':
            (data.sessions || []).forEach(s => { sessions[s.id] = s; });
            renderSessions(); updateSessionSelect(); break;
        case 'session_status':
            sessions[data.id] = sessions[data.id] || {};
            sessions[data.id].id = data.id;
            sessions[data.id].status = data.status;
            if (data.status !== 'waiting_qr') sessions[data.id].qr = null;
            renderSessions(); updateSessionSelect();
            addEvent({ event: `session.${data.status}`, session: data.id, data: {} });
            break;
        case 'qr':
            sessions[data.id] = sessions[data.id] || {};
            sessions[data.id].qr = data.qr;
            sessions[data.id].status = 'waiting_qr';
            renderSessions(); break;
        case 'event':
            addEvent(data);
            // Show incoming messages in chat
            if (data.event === 'message.received' && data.data) {
                const d = data.data;
                addWaBubble(d.preview || '[message]', 'in', d.name || d.from);
            }
            break;
    }
}

// ===== Sessions =====
function renderSessions() {
    const keys = Object.keys(sessions);
    sessionCountEl.textContent = `${keys.length} Session${keys.length !== 1 ? 's' : ''}`;
    if (keys.length === 0) {
        sessionsGrid.innerHTML = `<div class="empty-state"><svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="8" y1="12" x2="16" y2="12"/></svg><p>No sessions yet. Click "New Session" to start.</p></div>`;
        return;
    }
    sessionsGrid.innerHTML = keys.map(id => {
        const s = sessions[id], st = s.status || 'unknown';
        const qrHtml = s.qr ? `<div class="session-qr"><img src="${s.qr}" alt="QR" /></div>` : '';
        return `<div class="session-card">
      <div class="session-card-header">
        <span class="session-name">${esc(id)}</span>
        <span class="session-status ${st}"><span class="status-dot-mini"></span>${st}</span>
      </div>${qrHtml}
      <div class="session-actions">
        <button class="btn btn-sm" onclick="restartSession('${esc(id)}')">Restart</button>
        <button class="btn btn-sm btn-danger" onclick="deleteSession('${esc(id)}')">Delete</button>
      </div></div>`;
    }).join('');
}

function updateSessionSelect() {
    const keys = Object.keys(sessions);
    chatSessionSelect.innerHTML = '<option value="">Select session...</option>' +
        keys.map(id => `<option value="${esc(id)}">${esc(id)} (${sessions[id].status || '?'})</option>`).join('');
    // Auto-select first connected
    if (!chatSessionSelect.value) {
        for (const id of keys) {
            if (sessions[id].status === 'connected') {
                chatSessionSelect.value = id;
                break;
            }
        }
    }
}

// Session CRUD
document.getElementById('createSessionBtn').addEventListener('click', () => {
    createModal.style.display = 'flex';
    document.getElementById('newSessionId').focus();
});
document.getElementById('cancelCreate').addEventListener('click', () => { createModal.style.display = 'none'; });

document.getElementById('confirmCreate').addEventListener('click', async () => {
    const id = document.getElementById('newSessionId').value.trim();
    if (!id) return showToast('Session ID is required', 'error');
    try {
        const res = await fetch('/api/sessions', {
            method: 'POST', headers: authHeaders(),
            body: JSON.stringify({
                id,
                ignoreGroups: document.getElementById('optIgnoreGroups').checked,
                ignoreBroadcast: document.getElementById('optIgnoreBroadcast').checked,
                ignoreStatus: document.getElementById('optIgnoreStatus').checked,
                chatwootInboxId: (document.getElementById('optChatwootInboxId') || {}).value || '',
            }),
        });
        const data = await res.json();
        if (!data.success) throw new Error(data.error);
        createModal.style.display = 'none';
        document.getElementById('newSessionId').value = '';
        showToast('Session created! Scan QR code.', 'success');
    } catch (err) { showToast('Error: ' + err.message, 'error'); }
});

async function deleteSession(id) {
    if (!confirm(`Delete session "${id}"?`)) return;
    try {
        const res = await fetch(`/api/sessions/${id}`, { method: 'DELETE', headers: authHeaders() });
        const data = await res.json();
        if (!data.success) throw new Error(data.error);
        delete sessions[id]; renderSessions(); updateSessionSelect();
        showToast('Session deleted', 'success');
    } catch (err) { showToast('Error: ' + err.message, 'error'); }
}

async function restartSession(id) {
    try {
        const res = await fetch(`/api/sessions/${id}/restart`, { method: 'POST', headers: authHeaders() });
        const data = await res.json();
        if (!data.success) throw new Error(data.error);
        showToast('Session restarting...', 'info');
    } catch (err) { showToast('Error: ' + err.message, 'error'); }
}

// ===== Chat Sending =====
async function sendMessage() {
    const sessionId = chatSessionSelect.value;
    if (!sessionId) return showToast('Pilih session dulu', 'error');
    const phone = chatPhone.value.replace(/\D/g, '');
    if (!phone || phone.length < 8) return showToast('Masukkan nomor HP yang valid', 'error');
    const chatId = getFullPhone();
    const msgType = chatMsgType.value;
    let endpoint, body, preview;

    switch (msgType) {
        case 'text': {
            const text = document.getElementById('chatText').value.trim();
            if (!text) return showToast('Tulis pesan dulu', 'error');
            endpoint = `/api/${sessionId}/messages/send-text`;
            body = { chatId, text };
            preview = text;
            document.getElementById('chatText').value = '';
            break;
        }
        case 'image': {
            const url = document.getElementById('chatImageUrl').value.trim();
            if (!url) return showToast('Masukkan URL gambar', 'error');
            const caption = document.getElementById('chatCaption').value.trim();
            endpoint = `/api/${sessionId}/messages/send-image`;
            body = { chatId, url, caption, mimetype: 'image/jpeg' };
            preview = '📷 ' + (caption || 'Image');
            break;
        }
        case 'document': {
            const url = document.getElementById('chatDocUrl').value.trim();
            if (!url) return showToast('Masukkan URL file', 'error');
            const filename = document.getElementById('chatDocName').value.trim() || 'document';
            endpoint = `/api/${sessionId}/messages/send-document`;
            body = { chatId, url, filename, mimetype: 'application/octet-stream' };
            preview = '📎 ' + filename;
            break;
        }
        case 'location': {
            const lat = parseFloat(document.getElementById('chatLat').value);
            const lng = parseFloat(document.getElementById('chatLng').value);
            if (isNaN(lat) || isNaN(lng)) return showToast('Lat dan Lng harus angka', 'error');
            endpoint = `/api/${sessionId}/messages/send-location`;
            body = { chatId, lat, lng };
            preview = `📍 ${lat.toFixed(4)}, ${lng.toFixed(4)}`;
            break;
        }
        case 'poll': {
            const title = document.getElementById('chatPollTitle').value.trim();
            const opts = document.getElementById('chatPollOptions').value.trim();
            if (!title || !opts) return showToast('Isi judul dan opsi poll', 'error');
            endpoint = `/api/${sessionId}/messages/send-poll`;
            body = { chatId, title, options: opts.split(',').map(o => o.trim()).filter(o => o), selectableCount: 1 };
            preview = '📊 ' + title;
            break;
        }
    }

    // Show outgoing bubble immediately
    addWaBubble(preview, 'out');

    try {
        const res = await fetch(endpoint, { method: 'POST', headers: authHeaders(), body: JSON.stringify(body) });
        const data = await res.json();
        if (!data.success) {
            addWaBubble('⚠ Gagal: ' + data.error, 'error-bubble');
            showToast('Gagal kirim: ' + data.error, 'error');
        } else {
            showToast('✓ Pesan terkirim', 'success');
        }
    } catch (err) {
        addWaBubble('⚠ ' + err.message, 'error-bubble');
        showToast('Network error', 'error');
    }
}

// Bind all send buttons
document.getElementById('sendBtn').addEventListener('click', sendMessage);
document.getElementById('sendImageBtn').addEventListener('click', sendMessage);
document.getElementById('sendDocBtn').addEventListener('click', sendMessage);
document.getElementById('sendLocBtn').addEventListener('click', sendMessage);
document.getElementById('sendPollBtn').addEventListener('click', sendMessage);

// Enter key for text
document.getElementById('chatText').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
});

// ===== Chat Bubbles =====
function addWaBubble(text, type, senderName) {
    const empty = chatMessages.querySelector('.wa-chat-empty');
    if (empty) empty.remove();
    const div = document.createElement('div');
    div.className = `wa-bubble ${type}`;

    const time = new Date().toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });

    if (type === 'in' && senderName) {
        div.innerHTML = `<div style="font-size:0.72rem;color:#00a884;font-weight:600;margin-bottom:2px;">${esc(senderName)}</div>${esc(text)}<span class="wa-time">${time}</span>`;
    } else if (type === 'out') {
        div.innerHTML = `${esc(text)}<span class="wa-time">${time} <span class="wa-check">✓✓</span></span>`;
    } else {
        div.innerHTML = esc(text);
    }
    chatMessages.appendChild(div);
    chatMessages.scrollTop = chatMessages.scrollHeight;
}

// ===== Event Monitor =====
function addEvent(data) {
    const empty = eventsList.querySelector('.empty-state');
    if (empty) empty.remove();
    eventCount++;
    eventCountEl.textContent = `${eventCount} event${eventCount !== 1 ? 's' : ''}`;
    const eventName = data.event || 'unknown';
    const badgeClass = eventName.includes('received') ? 'received' :
        eventName.includes('sent') ? 'sent' :
            eventName.includes('connected') ? 'connected' : 'status';
    const d = data.data || {};
    const preview = d.preview || d.from || d.message || JSON.stringify(d).substring(0, 60);
    const div = document.createElement('div');
    div.className = 'event-item';
    div.innerHTML = `<span class="event-badge ${badgeClass}">${esc(eventName)}</span>
    <span class="event-session">${esc(data.session || '-')}</span>
    <span class="event-content">${esc(preview)}</span>
    <span class="event-time">${new Date().toLocaleTimeString()}</span>`;
    eventsList.prepend(div);
    while (eventsList.children.length > 200) eventsList.removeChild(eventsList.lastChild);
}

document.getElementById('clearEvents').addEventListener('click', () => {
    eventCount = 0; eventCountEl.textContent = '0 events';
    eventsList.innerHTML = `<div class="empty-state"><svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg><p>Events will appear here</p></div>`;
});

function esc(str) { const d = document.createElement('div'); d.textContent = String(str || ''); return d.innerHTML; }

// ===== Start =====
connect();
