(function_declaration
  name: (identifier) @function.name) @function.definition

(generator_function_declaration
  name: (identifier) @function.name) @function.definition

(method_definition
  name: [(property_identifier) (identifier)] @method.name) @method.definition

(class_declaration
  name: (type_identifier) @class.name) @class.definition

(abstract_class_declaration
  name: (type_identifier) @class.name) @class.definition

(interface_declaration
  name: (type_identifier) @class.name) @class.definition

(import_statement
  source: (string) @import.path) @import.definition

(comment) @doc.comment

(lexical_declaration
  (variable_declarator
    name: (identifier) @module.name)) @module.declaration

(type_alias_declaration
  name: (type_identifier) @module.name) @module.declaration

(enum_declaration
  name: (identifier) @module.name) @module.declaration
