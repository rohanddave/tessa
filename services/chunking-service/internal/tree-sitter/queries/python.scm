(function_definition
  name: (identifier) @function.name) @function.definition

(class_definition
  name: (identifier) @class.name) @class.definition

(import_statement) @import.definition

(import_from_statement) @import.definition

(comment) @doc.comment

(assignment
  left: (identifier) @module.name) @module.declaration
