'use strict';

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.default = PathProvider;

var _Path = require('../Path');

var _Path2 = _interopRequireDefault(_Path);

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

function createNext(next, path) {
  return function (payload) {
    return new _Path2.default(path, payload);
  };
}

function PathProvider() {
  return function (context, functionDetails, payload, next) {
    if (functionDetails.outputs) {
      context.path = Object.keys(functionDetails.outputs).reduce(function (output, outputPath) {
        output[outputPath] = createNext(next, outputPath);

        return output;
      }, {});
    }

    return context;
  };
}
//# sourceMappingURL=Path.js.map