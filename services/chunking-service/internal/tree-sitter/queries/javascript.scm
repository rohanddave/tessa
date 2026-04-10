(function_declaration
  name: (identifier) @function.name) @function.definition

(generator_function_declaration
  name: (identifier) @function.name) @function.definition

(method_definition
  name: [(property_identifier) (identifier)] @method.name) @method.definition

(class_declaration
  name: (identifier) @class.name) @class.definition

(import_statement
  source: (string) @import.path) @import.definition

(comment) @doc.comment

(lexical_declaration
  (variable_declarator
    name: (identifier) @module.name)) @module.declaration

(variable_declaration
  (variable_declarator
    name: (identifier) @module.name)) @module.declaration
