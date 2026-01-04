let limit = 50;
let offset = 0;

const els = {
  rows: document.getElementById("rows"),
  refresh: document.getElementById("refresh"),
  prev: document.getElementById("prev"),
  next: document.getElementById("next"),
  page: document.getElementById("page"),
  state: document.getElementById("state"),
  jobType: document.getElementById("jobType"),
  q: document.getElementById("q"),
};

async function apiGet(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

async function apiPost(url) {
  const r = await fetch(url, { method: "POST" });
  if (!r.ok) throw new Error(await r.text());
}

function qs(obj) {
  const p = new URLSearchParams();
  Object.entries(obj).forEach(([k, v]) => {
    if (v) p.set(k, v);
  });
  return p.toString();
}

async function loadJobs() {
  const q = {
    limit,
    offset,
    state: els.state.value,
    jobType: els.jobType.value,
    q: els.q.value,
  };

  const data = await apiGet(`/jobs?${qs(q)}`);
  els.rows.innerHTML = "";

  data.items.forEach((j) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${j.jobId}</td>
      <td>${j.jobType}</td>
      <td>${j.state}</td>
      <td>${j.retryCount}/${j.maxRetries}</td>
      <td>${new Date(j.createdAt).toLocaleString()}</td>
      <td>${new Date(j.updatedAt).toLocaleString()}</td>
      <td>
        <button data-a="retry" data-id="${j.jobId}">Retry</button>
        <button data-a="dlq" data-id="${j.jobId}">DLQ</button>
      </td>
    `;
    els.rows.appendChild(tr);
  });

  els.page.textContent = String(offset / limit + 1);
}

els.rows.addEventListener("click", async (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  const id = b.dataset.id;
  const a = b.dataset.a;
  await apiPost(`/jobs/${id}/${a}`);
  loadJobs();
});

els.refresh.onclick = () => {
  offset = 0;
  loadJobs();
};

els.prev.onclick = () => {
  offset = Math.max(0, offset - limit);
  loadJobs();
};

els.next.onclick = () => {
  offset += limit;
  loadJobs();
};

setInterval(loadJobs, 2500);
loadJobs();
