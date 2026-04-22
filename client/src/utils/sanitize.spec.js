import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { JSDOM } from "jsdom";
import { sanitizeReportHTML } from "./sanitize";

describe("sanitizeReportHTML", () => {
  let cleanup = {};

  beforeEach(() => {
    const dom = new JSDOM("<!doctype html><html><body></body></html>", {
      url: "http://localhost/",
    });
    cleanup.window = global.window;
    cleanup.document = global.document;
    cleanup.DOMParser = global.DOMParser;
    cleanup.NodeFilter = global.NodeFilter;
    global.window = dom.window;
    global.document = dom.window.document;
    global.DOMParser = dom.window.DOMParser;
    global.NodeFilter = dom.window.NodeFilter;
  });

  afterEach(() => {
    global.window = cleanup.window;
    global.document = cleanup.document;
    global.DOMParser = cleanup.DOMParser;
    global.NodeFilter = cleanup.NodeFilter;
  });

  it("preserves trusted ECharts loader/runtime scripts for report previews", () => {
    const html = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <script id="oda-echarts-loader" src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"></script>
</head>
<body>
  <div class="chart-box" data-chart-id="chart_sales" style="height: 400px"></div>
  <script id="oda-chart-runtime">
    document.addEventListener('DOMContentLoaded', function() {
      var nodes = document.querySelectorAll('.chart-box[data-chart-id="chart_sales"]');
      nodes.forEach(function(el) {
        var chart = echarts.init(el);
        chart.setOption({ xAxis: { type: 'category', data: ['A'] }, yAxis: { type: 'value' }, series: [{ type: 'bar', data: [1] }] });
        window.addEventListener('resize', function() { chart.resize(); });
      });
    });
  </script>
</body>
</html>`;

    const sanitized = sanitizeReportHTML(html);

    expect(sanitized).toContain('id="oda-echarts-loader"');
    expect(sanitized).toContain('https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js');
    expect(sanitized).toContain('id="oda-chart-runtime"');
    expect(sanitized).toContain("echarts.init(");
    expect(sanitized).toContain("document.querySelectorAll('.chart-box[data-chart-id=\"chart_sales\"]')");
  });

  it("removes untrusted inline scripts", () => {
    const html = `<!DOCTYPE html><html><body>
      <script>alert('xss')</script>
      <div class="chart-box" data-chart-id="chart_sales"></div>
    </body></html>`;

    const sanitized = sanitizeReportHTML(html);
    expect(sanitized).not.toContain("alert('xss')");
  });
});
