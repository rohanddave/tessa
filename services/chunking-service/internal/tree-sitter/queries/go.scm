(function_declaration
  name: (identifier) @function.name) @function.definition

(method_declaration
  name: (field_identifier) @method.name) @method.definition

(type_declaration
  (type_spec
    name: (type_identifier) @class.name
    type: [(struct_type) (interface_type)])) @class.definition

(import_spec
  path: [(interpreted_string_literal) (raw_string_literal)] @import.path) @import.definition

(comment) @doc.comment

(var_declaration
  (var_spec
    name: (identifier) @module.name)) @module.declaration

(const_declaration
  (const_spec
    name: (identifier) @module.name)) @module.declaration

(type_declaration
  (type_spec
    name: (type_identifier) @module.name)) @module.declaration
