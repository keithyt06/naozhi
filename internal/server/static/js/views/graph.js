// js/views/graph.js — Phase 4A Knowledge Graph visualization.
//
// Moved verbatim from dashboard.html's inline <script>
// (/* ===== Phase 4A: Knowledge Graph Visualization ===== */ block).
// d3 is lazily imported on first mount, not at module load time — so
// the ~250KB d3 bundle only ships when the user opens the graph view.
//
// Shared helpers (esc/escJs/authHeaders) are reached via window
// bridges because the legacy inline script still owns them. When
// Phase 2 collapses the legacy bridge, swap to direct imports.

var graphData = null;
var graphSim = null;
var graphSelectedNode = null;
var graphTypeFilter = 'all';
var graphD3 = null;

var GRAPH_COLORS = {
  hub: '#a376ff', service: '#58a6ff', project: '#3fb950',
  infra: '#d29922', security: '#da3633'
};
var GRAPH_LABELS = {
  hub: 'Knowledge Hub', service: 'AWS Service', project: 'Project',
  infra: 'Infrastructure', security: 'Security'
};

async function loadD3() {
  if (graphD3) return graphD3;
  try {
    graphD3 = await import('https://cdn.jsdelivr.net/npm/d3@7/+esm');
  } catch (e) {
    console.warn('d3 CDN failed', e);
    return null;
  }
  return graphD3;
}

function renderGraphView() {
  const main = document.getElementById('main');
  main.innerHTML =
    '<div class="graph-container" id="graphContainer">' +
      '<svg class="graph-svg" id="graphSvg"></svg>' +
      '<div class="graph-filter-bar" id="graphFilterBar">' +
        '<button class="graph-filter-pill active" data-type="all" onclick="filterGraphType(\'all\',this)">All</button>' +
        '<button class="graph-filter-pill" data-type="hub" onclick="filterGraphType(\'hub\',this)">Hub</button>' +
        '<button class="graph-filter-pill" data-type="service" onclick="filterGraphType(\'service\',this)">Service</button>' +
        '<button class="graph-filter-pill" data-type="project" onclick="filterGraphType(\'project\',this)">Project</button>' +
        '<button class="graph-filter-pill" data-type="infra" onclick="filterGraphType(\'infra\',this)">Infra</button>' +
        '<button class="graph-filter-pill" data-type="security" onclick="filterGraphType(\'security\',this)">Security</button>' +
      '</div>' +
      '<div class="graph-legend">' +
        Object.keys(GRAPH_COLORS).map(function(t) {
          return '<div class="graph-legend-item"><div class="graph-legend-dot" style="background:' + GRAPH_COLORS[t] + '"></div>' + GRAPH_LABELS[t] + '</div>';
        }).join('') +
      '</div>' +
      '<div class="graph-detail" id="graphDetail"></div>' +
    '</div>';
  loadGraphData();
}

async function loadGraphData() {
  try {
    const resp = await fetch('/api/graph', { headers: authHeaders() });
    if (!resp.ok) throw new Error('Graph API error');
    graphData = await resp.json();
    if (!graphData.nodes || graphData.nodes.length === 0) {
      document.getElementById('graphContainer').innerHTML =
        '<div class="wv-empty"><span style="font-size:28px;opacity:.3">&#128200;</span><span>No graph data yet. Run Wiki Ingest first.</span></div>';
      return;
    }
    initGraphSvg();
  } catch (e) {
    document.getElementById('graphContainer').innerHTML =
      '<div class="wv-empty"><span style="color:#f85149">' + esc(e.message) + '</span></div>';
  }
}

async function initGraphSvg() {
  var d3 = await loadD3();
  if (!d3 || !graphData) return;

  var container = document.getElementById('graphContainer');
  var svg = d3.select('#graphSvg');
  var width = container.clientWidth;
  var height = container.clientHeight;
  svg.attr('viewBox', [0, 0, width, height]);

  // Clear previous
  svg.selectAll('*').remove();
  var g = svg.append('g');

  // Zoom
  var zoom = d3.zoom().scaleExtent([0.1, 5]).on('zoom', function(event) {
    g.attr('transform', event.transform);
  });
  svg.call(zoom);

  // Click on empty space to deselect
  svg.on('click', function(event) {
    if (event.target === svg.node()) {
      graphSelectedNode = null;
      document.getElementById('graphDetail').classList.remove('open');
      linkG.selectAll('line').attr('stroke-opacity', 0.3);
      nodeG.selectAll('circle').attr('opacity', 1);
      labelG.selectAll('text').attr('opacity', 1);
    }
  });

  var nodes = graphData.nodes.map(function(n) { return Object.assign({}, n); });
  var edges = graphData.edges.map(function(e) { return { source: e.source, target: e.target, weight: e.weight, label: e.label }; });

  // Filter out edges whose source/target not in nodes
  var nodeIds = new Set(nodes.map(function(n) { return n.id; }));
  edges = edges.filter(function(e) { return nodeIds.has(e.source) && nodeIds.has(e.target); });

  var sim = d3.forceSimulation(nodes)
    .force('link', d3.forceLink(edges).id(function(d) { return d.id; }).distance(80))
    .force('charge', d3.forceManyBody().strength(-200))
    .force('center', d3.forceCenter(width / 2, height / 2))
    .force('collide', d3.forceCollide().radius(function(d) { return nodeRadius(d) + 4; }));

  if (nodes.length > 200) sim.alphaDecay(0.05);
  graphSim = sim;

  var linkG = g.append('g');
  var link = linkG.selectAll('line')
    .data(edges).join('line')
    .attr('stroke', '#484f58').attr('stroke-opacity', 0.3)
    .attr('stroke-width', function(d) { return Math.max(1, d.weight); });

  var nodeG = g.append('g');
  var node = nodeG.selectAll('circle')
    .data(nodes).join('circle')
    .attr('r', nodeRadius)
    .attr('fill', function(d) { return d.color || GRAPH_COLORS[d.type] || '#6B7280'; })
    .attr('stroke', '#0d1117').attr('stroke-width', 1.5)
    .attr('cursor', 'pointer')
    .on('click', function(event, d) {
      event.stopPropagation();
      selectGraphNode(d);
    })
    .call(d3.drag()
      .on('start', function(event, d) { if (!event.active) sim.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y; })
      .on('drag', function(event, d) { d.fx = event.x; d.fy = event.y; })
      .on('end', function(event, d) { if (!event.active) sim.alphaTarget(0); d.fx = null; d.fy = null; })
    );

  var labelG = g.append('g');
  var label = labelG.selectAll('text')
    .data(nodes).join('text')
    .text(function(d) { return d.label; })
    .attr('font-size', 10).attr('fill', '#8b949e')
    .attr('text-anchor', 'middle').attr('dy', function(d) { return nodeRadius(d) + 12; })
    .attr('pointer-events', 'none');

  sim.on('tick', function() {
    link.attr('x1', function(d) { return d.source.x; }).attr('y1', function(d) { return d.source.y; })
        .attr('x2', function(d) { return d.target.x; }).attr('y2', function(d) { return d.target.y; });
    node.attr('cx', function(d) { return d.x; }).attr('cy', function(d) { return d.y; });
    label.attr('x', function(d) { return d.x; }).attr('y', function(d) { return d.y; });
  });
}

function nodeRadius(d) {
  return Math.max(8, Math.min(24, 4 + (d.size || 0) * 2));
}

function filterGraphType(type, el) {
  graphTypeFilter = type;
  document.querySelectorAll('#graphFilterBar .graph-filter-pill').forEach(function(b) {
    b.classList.toggle('active', b.dataset.type === type);
  });
  if (!graphD3) return;
  var svg = graphD3.select('#graphSvg');
  svg.selectAll('circle').attr('opacity', function(d) {
    return (type === 'all' || d.type === type) ? 1 : 0.1;
  });
  svg.selectAll('text').attr('opacity', function(d) {
    return (type === 'all' || d.type === type) ? 1 : 0.05;
  });
}

function selectGraphNode(d) {
  graphSelectedNode = d;
  // Highlight neighbors
  if (graphD3 && graphData) {
    var neighbors = new Set();
    neighbors.add(d.id);
    graphData.edges.forEach(function(e) {
      if (e.source === d.id || (e.source && e.source.id === d.id)) {
        neighbors.add(typeof e.target === 'string' ? e.target : e.target.id);
      }
      if (e.target === d.id || (e.target && e.target.id === d.id)) {
        neighbors.add(typeof e.source === 'string' ? e.source : e.source.id);
      }
    });
    graphD3.select('#graphSvg').selectAll('circle').attr('opacity', function(n) {
      return neighbors.has(n.id) ? 1 : 0.15;
    });
    graphD3.select('#graphSvg').selectAll('line').attr('stroke-opacity', function(e) {
      var sid = typeof e.source === 'string' ? e.source : e.source.id;
      var tid = typeof e.target === 'string' ? e.target : e.target.id;
      return (sid === d.id || tid === d.id) ? 0.8 : 0.05;
    });
  }
  showGraphDetail(d);
}

function showGraphDetail(d) {
  var panel = document.getElementById('graphDetail');
  var conns = [];
  if (graphData) {
    graphData.edges.forEach(function(e) {
      var sid = typeof e.source === 'string' ? e.source : e.source.id;
      var tid = typeof e.target === 'string' ? e.target : e.target.id;
      if (sid === d.id) conns.push(tid);
      else if (tid === d.id) conns.push(sid);
    });
  }
  var typeBg = (GRAPH_COLORS[d.type] || '#6B7280') + '33';
  var typeColor = GRAPH_COLORS[d.type] || '#6B7280';
  panel.innerHTML =
    '<button style="float:right;background:none;border:none;color:#8b949e;cursor:pointer;font-size:16px" onclick="document.getElementById(\'graphDetail\').classList.remove(\'open\')">&times;</button>' +
    '<h3>' + esc(d.label) + '</h3>' +
    '<span class="graph-detail-type" style="background:' + typeBg + ';color:' + typeColor + '">' + esc(GRAPH_LABELS[d.type] || d.type) + '</span>' +
    '<div style="margin-top:12px;font-size:12px;color:#8b949e">' + conns.length + ' connections</div>' +
    (conns.length > 0 ? '<ul class="graph-detail-conns">' + conns.map(function(c) {
      return '<li onclick="focusGraphNode(\'' + escJs(c) + '\')">' + esc(c) + '</li>';
    }).join('') + '</ul>' : '') +
    (d.wiki_page ? '<button class="graph-btn" onclick="switchView(\'wiki\',document.querySelector(\'[data-view=wiki]\'));setTimeout(function(){loadWikiPage(\'' + escJs(d.wiki_page) + '\')},100)">Open Wiki</button>' : '');
  panel.classList.add('open');
}

function focusGraphNode(id) {
  if (!graphData) return;
  var found = graphData.nodes.find(function(n) { return n.id === id; });
  if (found) selectGraphNode(found);
}

// Shared helpers reached via legacy window bridges — removed after Phase 2.
function authHeaders() {
  return (typeof window !== 'undefined' && typeof window.authHeaders === 'function')
    ? window.authHeaders() : {};
}
function esc(s) {
  return (typeof window !== 'undefined' && typeof window.esc === 'function')
    ? window.esc(s) : String(s == null ? '' : s);
}
function escJs(s) {
  return (typeof window !== 'undefined' && typeof window.escJs === 'function')
    ? window.escJs(s) : String(s == null ? '' : s).replace(/'/g, "\\'");
}

// ------- Legacy bridges — removed after Phase 2 -------------------

if (typeof window !== 'undefined') {
  window.renderGraphView = renderGraphView;
  window.loadGraphData = loadGraphData;
  window.initGraphSvg = initGraphSvg;
  window.filterGraphType = filterGraphType;
  window.selectGraphNode = selectGraphNode;
  window.showGraphDetail = showGraphDetail;
  window.focusGraphNode = focusGraphNode;
  window.GRAPH_COLORS = GRAPH_COLORS;
  window.GRAPH_LABELS = GRAPH_LABELS;
}
