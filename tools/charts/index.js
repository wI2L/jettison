#!/usr/bin/env node

const fs = require('fs')
const jd = require('jsdom')
const {
    JSDOM
} = jd
const path = require('path')
const prog = require('commander')
const util = require('util')
const deca = require('decamelize')
const SVGO = require('svgo')

prog
    .option('-f, --input <file>', 'json-formatted benchmark data file')
    .option('-d, --directory <dir>', 'output directory of charts')
    .option('-n, --names <list>', 'comma-separated list of benchmark names', (value) => {
        return value.split(',')
    })

prog.version('0.2.0')
prog.parse(process.argv)

// createChart creates and exports a Google Bar Chart
// as a PNG image, using the decamelized form of name
// with '-' as separator. The parameter data must be a
// two-dimensional array where each row represents a
// bar in the chart.
function createBarChart (dom, name, data, opts) {
    var head = dom.window.document.getElementsByTagName('head')[0]
    var func = head.insertBefore

    // Prevent call to Google Font API.
    head.insertBefore = function (el, ref) {
        if (el.href && el.href.indexOf('//fonts.googleapis.com/css?family=Input') > -1) {
            return
        }
        func.call(head, el, ref)
    }
    // Add an event listener to draw the chart once
    // the DOM is fully loaded.
    dom.window.addEventListener('DOMContentLoaded', function () {
        const g = dom.window.google

        // Load the Google Visualization
        // API and the corechart package.
        g.charts.load('45.2', {
            packages: ['corechart', 'bar']
        })
        g.charts.setOnLoadCallback(function () {
            drawBarChart(dom, name, data, opts)
        })
    })
}

// exportChartAsSVG exports the SVG of the chart to
// the current working directory. The file name is a
// decamelized version of name using the hyphen char
// as separator, in lowercase.
function exportChartAsSVG (e, name) {
    var filename = util.format('%s.svg',
        path.join(prog.directory, deca(name, '-'))
    )
    var svgEl = e.getElementsByTagName('svg')[0]
    var svg = htmlToElement(svgEl.outerHTML)

    svg.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
    svg.setAttribute('version', '1.1')

    var svgo = new SVGO({
        plugins: [{
            sortAttrs: true
        }, {
            removeAttrs: {
                attrs: '(clip-path|aria-label|overflow)'
            }
        }]
    })
    svgo.optimize(svg.outerHTML, {}).then(function (result) {
        try {
            return fs.writeFileSync(filename, result.data)
        } catch (err) {
            console.error('cannot write svg chart %s: %s', filename, err)
        }
    })
}

function htmlToElement (html) {
    const d = (new JSDOM('...')).window.document
    var t = d.createElement('template')
    t.innerHTML = html.trim()
    return t.content.firstChild
}

function drawBarChart (dom, name, data, opts) {
    const g = dom.window.google
    const d = dom.window.document

    // Convert the data 2D-array to a DataTable,
    // and sort it in alphabetical order using
    // the content of the first column which
    // contains the names of the benchmark runs.
    var dt = new g.visualization.arrayToDataTable(data)
    dt.sort([{
        column: 0
    }])
    var e = d.getElementById('chart')
    var c = new g.visualization.ColumnChart(e)

    // Setup a callback that exports the chart as
    // a PNG image when it has finished drawing.
    g.visualization.events.addListener(c, 'ready', function () {
        exportChartAsSVG(e, name)
    })
    c.draw(dt, opts)
}

// Load and parse the JSON-formatted benchmark
// statistics from the input file.
var file = fs.readFileSync(prog.input)
var data = JSON.parse(file)

// Iterate over all benchmarks and create the
// chart only if it was requested through the
// command-line parameters.
Object.keys(data).forEach(function (key) {
    if (!prog.names.includes(key)) {
        return
    }
    const pageFile = path.join(__dirname, 'jsdom.html')

    JSDOM.fromFile(pageFile, {
        resources: 'usable',
        runScripts: 'dangerously',
        pretendToBeVisual: true
    }).then(dom => {
        createBarChart(dom, key, data[key], {
            width: 700,
            height: 400,
            chartArea: {
                left: 100,
                top: 50,
                width: '70%',
                height: '75%'
            },
            vAxis: {
                format: '',
                gridlines: {
                    count: 5
                },
                minorGridlines: {
                    count: 2
                }
            },
            hAxis: {
                textStyle: {
                    bold: true,
                    fontName: 'Input'
                }
            }
        })
    })
})
