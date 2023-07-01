const { Marp } = require('@marp-team/marp-core')

const marpHideSlidesPlugin = require('./hide-slides-plugin')

const markdownItIncludeOptions = {
    includeRe: /@include(.+)/
  };

module.exports = function(opts) {
  opts['html'] = true;
  opts['output'] = './dist';

  return new Marp(opts).
    use(require('markdown-it-attrs')).  
    use(require('markdown-it-container'), 'columns').
    use(require('markdown-it-include'), markdownItIncludeOptions).
    use(marpHideSlidesPlugin);
}
