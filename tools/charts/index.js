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

prog
    .option('-f, --input <file>', 'json-formatted benchmark data file')
    .option('-d, --directory <dir>', 'output directory of charts')
    .option('-n, --names <list>', 'comma-separated list of benchmark names', (value) => {
        return value.split(',')
    })

prog.version('0.0.1')
prog.parse(process.argv)

// createChart creates and exports a Google Bar Chart
// as a PNG image, using the decamelized form of name
// with '-' as seperator. The parameter data must be a
// two-dimentional array where each row represents a
// bar in the chart.
function createBarChart (dom, name, data, opts) {
    // Add an event listener to draw the chart once
    // the DOM is fully loaded.
    dom.window.addEventListener('DOMContentLoaded', function () {
        const g = dom.window.google

        // Load the Google Visualization API and
        // the corechart package.
        // Use version 45, because 'current' has
        // issues when rendering Y-axis labels.
        g.charts.load('45', {
            packages: ['corechart', 'bar']
        })
        g.charts.setOnLoadCallback(function () {
            drawBarChart(dom, name, data, opts)
        })
    })
}

// exportChart exports the PNG image of the chart
// to the current working directory. The file name
// is a decamelized version of name using the hyphen
// character as separator, in lowercase.
function exportChart (chart, name) {
    var filename = util.format('%s.png',
        path.join(prog.directory, deca(name, '-'))
    )
    var img = chart.getImageURI().replace(
        /^data:image\/png;base64,/, ''
    )
    try {
        return fs.writeFileSync(filename, img, 'base64')
    } catch (err) {
        console.error('cannot write %s chart: %s', filename, err)
    }
    console.log('chart %s exported', filename)
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
        exportChart(c, name)
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
for (var key in data) {
    if (!prog.names.includes(key)) {
        continue
    }
    const pageFile = path.join(__dirname, 'jsdom.html')

    JSDOM.fromFile(pageFile, {
        resources: 'usable',
        runScripts: 'dangerously',
        pretendToBeVisual: true
    }).then(dom => {
        createBarChart(dom, key, data[key], {
            width: 580,
            height: 400,
            vAxis: {
                format: '',
                gridlines: {
                    count: 5
                },
                minorGridlines: {
                    count: 1
                }
            },
            hAxis: {
                textStyle: {
                    bold: true,
                    fontName: 'Lato'
                }
            }
        })
    })
}
