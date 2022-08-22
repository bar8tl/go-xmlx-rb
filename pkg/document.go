/*
Este paquete es una capa adicional a la libreria estandard XML y la usa para
construir un arbol de nodos desde cualquier documento que usted cargue. Esto le
permitira buscar nodos hacia adelante y hacia atras, asi como ejecutar queries
simples de busqueda.

De esta manera, los nodos se convierten simplemente en colecciones y no
requieren que los lea en el orden en que el xml.Parser los encuentra.

El archivo "Document" implementa hasta ahora 2 funciones de busqueda que le
permitiran buscar nodos especificos

*xmlx.Document.SelectNode          (namespace, name string)   *Node;
*xmlx.Document.SelectNodes         (namespace, name string) []*Node;
*xmlx.Document.SelectNodesRecursive(namespace, name string) []*Node;

SelectNode () devuelve el primer o unico nodo que se encuentra al buscar por un
nombre y namespace dados.
SelectNodes() devuelve una seccion conteniendo todos los nodos coincidentes
(sin entrar recursivamente en los nodos coincidentes)
SelectNodesRecursive() devuelve una seccion con todos los nodos coincidentes,
incluyendo los nodos dentro de otros nodos coincidentes

Note que estas funciones de busqueda pueden ser llamadas en nodos individuales
tambien. Esto le permitira buscar solo un subconjunto del documento entero.
*/

package xmlx

import (
  "bytes"
  "encoding/xml"
  "errors"
  "fmt"
  "io"
  "io/ioutil"
  "net/http"
  "os"
  "strings"
)

// Esta firma representa una rutina de conversion de estandares de codificacion
// de caracteres.
// Se usa para indicar al decodificador XML como tratar caracteres no UTF-8.
type CharsetFunc func(charset string, input io.Reader) (io.Reader, error)

// Este tipo representa un documento XML simple.
type Document struct {
  Version     string             // Version XML
  Encoding    string             // Tipo de codificacion encontrado en el documento. Si no existiera se asume UTF-8.
  StandAlone  string             // Valor del atributo 'standalone' del doctype XML.
  Entity      map[string]string  // Mapeo de conversiones de entidades de configuracion.
  Root       *Node               // El nodo raiz del documento.
  SaveDocType bool               // Indicador de incluir o no los doctype XML al salvar el documento
  Namespaces  map[string]string  // Mapa de namespaces del documento
}

// Funcion para crear una instancia nueva y vacia de documento XML.
func New() *Document {
  return &Document{
    Version:     "1.0",
    Encoding:    "UTF-8",
    StandAlone:  "yes",
    SaveDocType: true,
    Entity:      make(map[string]string),
    Namespaces:  make(map[string]string),
  }
}

// Esta funcion carga una tabla masiva de secuencias de escape XML no
// convencionales.
// Se necesita para hacer que el parser las mapee apropiadamente. Se aconseja
// establecer manualmente solo aquellas entidades
// necesarias usando el map document.Entity, pero de ser necesario puede ser
// tambien llamada para llenar el mapa con el conjunto
// entero que esta definido en http://www.w3.org/TR/html4/sgml/entities.html
func (this *Document) LoadExtendedEntityMap() {
  loadNonStandardEntities(this.Entity)
}

// Selecciona un nodo simple con un nombre y namespace dados. Devuelve 'nil'
// si no se encuentra un nodo que haga match.
func (this *Document) SelectNode(namespace, name string) *Node {
    return this.Root.SelectNode(namespace, name)
}

// Selecciona todos los nodos con un nombre y namespace dados. Devuelve una
// seccion vacia si no se encuentran nodos que hagan match.
// Hace la seleccion sin hacer recursion en los nodos hijo de los nodos que 
//hacen match.
func (this *Document) SelectNodes(namespace, name string) []*Node {
  return this.Root.SelectNodes(namespace, name)
}

// Selecciona todos los nodos con un nombre y namespace dados. Hace la
// seleccion tambien entrando recursivamente a los nodos hijo de los nodos que
// hacen match. Devuelve una seccion o slice vacia si no hay nodos que hagan
// match.
func (this *Document) SelectNodesRecursive(namespace, name string) []*Node {
  return this.Root.SelectNodesRecursive(namespace, name)
}

// Carga el contenido de este documento desde el reader proporcionado.
func (this *Document) LoadStream(r io.Reader, charset CharsetFunc) (err error) {
  xp := xml.NewDecoder(r)          // Tipo de retorno: *Decoder <-- Crea un parser XMl desde el reader r
  xp.Entity = this.Entity          // Asigna al parser el area de memoria para mapa de entidades del documento
  xp.CharsetReader = charset       // Crea una instancia de la funcion de mapeo para el parser

  this.Root = NewNode(NT_ROOT)
  ct := this.Root                  // Tipo *Node - corresponde al current node

  var tok xml.Token
  var t *Node
  var doctype string
    
  for {
    if tok, err = xp.Token(); err != nil {
      if err == io.EOF {
        return nil
      }
      return err
    }

    switch tt := tok.(type) {
    case xml.SyntaxError:
      return errors.New(tt.Error())
    case xml.CharData:
      t := NewNode(NT_TEXT)
      t.Value = string([]byte(tt))
      ct.AddChild(t)
    case xml.Comment:
      t := NewNode(NT_COMMENT)
      t.Value = strings.TrimSpace(string([]byte(tt)))
      ct.AddChild( t )
    case xml.Directive:
      t = NewNode(NT_DIRECTIVE)
      t.Value = strings.TrimSpace(string([]byte(tt)))
      ct.AddChild(t)
    case xml.StartElement:
      t = NewNode(NT_ELEMENT)
      t.Name = tt.Name
      t.Attributes = make([]*Attr, len(tt.Attr))
      for i, v := range tt.Attr {
        if v.Name.Space == "" && v.Name.Local == "xmlns" {                  // Crear mapa de namespaces
          this.Namespaces[v.Value] = ""                                     // ...
        } else if v.Name.Space == "xmlns" {                                 // ...
          this.Namespaces[v.Value] = v.Name.Local                           // ...
        }                                                                   // ...
        t.Attributes[i] = new(Attr)
        t.Attributes[i].Name = v.Name
        t.Attributes[i].Value = v.Value
        if alias, ok := this.Namespaces[t.Attributes[i].Name.Space]; ok {   // ...
          t.Attributes[i].Name.Space = alias                                // ...
        }                                                                   // ...
      }                                                                     // ...
      if alias, ok := this.Namespaces[t.Name.Space]; ok {                   // ...
        t.Name.Space = alias                                                // ...
      }                                                                     // ...
      ct.AddChild( t )
      ct = t
    case xml.ProcInst:
      if tt.Target == "xml" { // xml doctype
        doctype = strings.TrimSpace(string(tt.Inst))
        if i := strings.Index(doctype, `standalone="`); i > -1 {
          this.StandAlone = doctype[i+len(`standalone="`) : len(doctype)]
          i = strings.Index(this.StandAlone, `"`) 
          this.StandAlone = this.StandAlone[0:i] 
        }
      } else {
        t = NewNode(NT_PROCINST)
        t.Target = strings.TrimSpace(tt.Target)
        t.Value = strings.TrimSpace(string(tt.Inst))
        ct.AddChild(t)
      }
    case xml.EndElement:
      if ct = ct.Parent; ct == nil {
        return
      }
    }
  }
  return
}

// Carga el contenido de este documento desde la seccion de bytes proporcionada.
func (this *Document) LoadBytes( d []byte, charset CharsetFunc ) (err error) {
  return this.LoadStream( bytes.NewBuffer( d ), charset )
}

// Carga el contenido de este documento desde el string proporcionado.
func (this *Document) LoadString( s string, charset CharsetFunc ) (err error) {
  return this.LoadStream( strings.NewReader(s), charset )
}

// Carga el contenido de este documento desde el archivo proporcionado.
func (this *Document) LoadFile( filename string, charset CharsetFunc ) (err error) {
  var fd *os.File
  if fd, err = os.Open( filename ); err != nil {
    return
  }
  defer fd.Close( )
  return this.LoadStream( fd, charset )
}

// Carga el contenido de este documento desde la URI proporcionads usando el cliente especificado.
func (this *Document) LoadUriClient( uri string, client *http.Client, charset CharsetFunc ) (err error) {
  var r *http.Response
  if r, err = client.Get( uri ); err != nil {
    return
  }
  defer r.Body.Close( )
  return this.LoadStream( r.Body, charset )
}

// Carga el contenido de este documento desde el URI proporcionado. (llama a LoadUriClient con http.DefaultClient).
func (this *Document) LoadUri( uri string, charset CharsetFunc ) (err error) {
  return this.LoadUriClient( uri, http.DefaultClient, charset )
}

// Salva el contenido de este documento en el archivo proporcionado.
func (this *Document) SaveFile( path string ) error {
  return ioutil.WriteFile( path, this.SaveBytes( ), 0600 )
}

// Salva el contenido de este documento como una seccion de bytes.
func (this *Document) SaveBytes( ) []byte {
  var b bytes.Buffer

  if this.SaveDocType {
    b.WriteString( fmt.Sprintf(`<?xml version="%s" encoding="%s" standalone="%s"?>`, this.Version, this.Encoding, this.StandAlone) )
    if len( IndentPrefix ) > 0 {
      b.WriteByte( '\n' )
    }
  }
  b.Write( this.Root.Bytes( ) )
  return b.Bytes( )
}

// Salva el contenido de este documento como un string.
func (this *Document) SaveString( ) string {
  return string( this.SaveBytes( ) )
}

// Alias de Document.SaveString().
// Esta funcion es invocada por todo lo que se refiera al metodo estandar String( ) (ej: fmt.Printf("%s\n", mydoc).
func (this *Document) String( ) string {
  return string( this.SaveBytes( ) )
}

// Salva el contenido de este documento en el writer proporcionado.
func (this *Document) SaveStream( w io.Writer ) (err error) {
  _, err = w.Write( this.SaveBytes( ) )
  return
}