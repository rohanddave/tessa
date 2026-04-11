(method_declaration
  name: (identifier) @method.name) @method.definition

(constructor_declaration
  name: (identifier) @method.name) @method.definition

(class_declaration
  name: (identifier) @class.name) @class.definition

(interface_declaration
  name: (identifier) @class.name) @class.definition

(enum_declaration
  name: (identifier) @class.name) @class.definition

(import_declaration
  [(scoped_identifier) (identifier)] @import.path) @import.definition

(line_comment) @doc.comment

(block_comment) @doc.comment

(field_declaration
  (variable_declarator
    name: (identifier) @module.name)) @module.declaration
