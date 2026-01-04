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
  kTotal: document.getElementById("kTotal"),
  kPending: document.getElementById("kPending"),
  kFailed: document.getElementById("kFailed"),
  kDLQ: document.getElementById("kDLQ"),
  chart: document.getElementById("chart"),
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
    if (v !== undefined && v !== null && String(v).trim() !== "") {
      p.set(k, v);
    }
  });
  return p.toString();
}

let chart = null;
const statsHistory = [];
const maxPoints = 120;

function isoToLabel(iso) {
  const d = new Date(iso);
  return d.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function ensureChart() {
  if (chart) return;

  chart = new Chart(els.chart, {
    type: "line",
    data: {
      labels: [],
      datasets: [
        {
          label: "Total jobs",
          data: [],
          tension: 0.25,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      plugins: {
        legend: { display: true },
      },
      scales: {
        y: { beginAtZero: true },
      },
    },
  });
}

function pushStatsPoint(p) {
  statsHistory.push(p);
  while (statsHistory.length > maxPoints) statsHistory.shift();
}

function renderChart() {
  ensureChart();

  chart.data.labels = statsHistory.map((p) => isoToLabel(p.ts));
  chart.data.datasets[0].data = statsHistory.map((p) => p.total);
  chart.update();
}

async function loadStats() {
  const data = await apiGet("/stats");

  els.kTotal.textContent = String(data.total ?? 0);
  els.kPending.textContent = String(data.pending ?? 0);
  els.kFailed.textContent = String(data.failed ?? 0);
  els.kDLQ.textContent = String(data.dlq ?? 0);

  if (Array.isArray(data.points) && data.points.length > 0) {
    const p = data.points[0];
    pushStatsPoint({
      ts: p.ts,
      total: p.total ?? 0,
    });
    renderChart();
  }
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

async function refreshAll() {
  await Promise.allSettled([loadJobs(), loadStats()]);
}

els.rows.addEventListener("click", async (e) => {
  const b = e.target.closest("button");
  if (!b) return;

  const id = b.dataset.id;
  const a = b.dataset.a;

  try {
    await apiPost(`/jobs/${id}/${a}`);
  } catch (err) {
    console.error(err);
  }

  refreshAll();
});

els.refresh.onclick = () => {
  offset = 0;
  refreshAll();
};

els.prev.onclick = () => {
  offset = Math.max(0, offset - limit);
  refreshAll();
};

els.next.onclick = () => {
  offset += limit;
  refreshAll();
};

els.state.onchange = () => {
  offset = 0;
  loadJobs();
};

els.jobType.onkeydown = (e) => {
  if (e.key === "Enter") {
    offset = 0;
    loadJobs();
  }
};

els.q.onkeydown = (e) => {
  if (e.key === "Enter") {
    offset = 0;
    loadJobs();
  }
};

setInterval(refreshAll, 2500);
refreshAll();
