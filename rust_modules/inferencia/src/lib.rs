use c_str_macro::c_str;
use rsmgp_sys::memgraph::Memgraph;
use rsmgp_sys::rsmgp::Type;
use rsmgp_sys::result::Result;
use rsmgp_sys::value::Value;
use rsmgp_sys::{close_module, define_procedure, define_type, init_module};
use std::collections::{HashMap, VecDeque};
use std::ffi::CString;
use std::os::raw::c_int;
use std::panic;

// Import necessary types for macros
use rsmgp_sys::mgp::{mgp_graph, mgp_list, mgp_memory, mgp_module, mgp_result};
use rsmgp_sys::rsmgp::{set_memgraph_error_msg, NamedType};

struct NodeEnergy {
    id: i64,
    energy: f64,
}

define_procedure!(optimize_context, |memgraph: &Memgraph| -> Result<()> {
    let args = memgraph.args()?;
    let start_node_ids_val = args.value_at(0)?;
    let decay_factor_val = args.value_at(1)?;
    let min_threshold_val = args.value_at(2)?;

    let start_node_ids: Vec<i64> = match start_node_ids_val {
        Value::List(list) => {
            let mut ids = Vec::new();
            for val in list.iter()? {
                if let Value::Int(id) = val {
                    ids.push(id);
                }
            }
            ids
        },
        _ => return Ok(()),
    };

    let decay_factor = match decay_factor_val {
        Value::Float(f) => f,
        Value::Int(i) => i as f64,
        _ => 0.8,
    };

    let min_threshold = match min_threshold_val {
        Value::Float(f) => f,
        Value::Int(i) => i as f64,
        _ => 0.2,
    };

    // Map to track the highest energy found for each node
    let mut energy_map: HashMap<i64, f64> = HashMap::new();
    let mut queue: VecDeque<NodeEnergy> = VecDeque::new();

    for id in start_node_ids {
        energy_map.insert(id, 1.0);
        queue.push_back(NodeEnergy { id, energy: 1.0 });
    }

    let mut iterations = 0;
    let max_iterations = 10000;

    while let Some(current) = queue.pop_front() {
        iterations += 1;
        if iterations > max_iterations {
            break;
        }

        if current.energy < min_threshold {
            continue;
        }

        let vertex = match memgraph.vertex_by_id(current.id) {
            Ok(v) => v,
            Err(_) => continue,
        };

        let edges = match vertex.out_edges() {
            Ok(iter) => iter,
            Err(_) => continue,
        };

        for edge in edges {
            let edge = edge;
            // Use to_vertex instead of target_vertex
            let target_vertex = match edge.to_vertex() {
                Ok(v) => v,
                Err(_) => continue,
            };
            let target_id = target_vertex.id();

            let weight_prop_name = c_str!("weight");
            let weight_prop = edge.property(&weight_prop_name);
            
            let edge_weight = match weight_prop {
                Ok(prop) => {
                    match prop.value {
                        Value::Float(w) => w,
                        Value::Int(w) => w as f64,
                        _ => 0.5,
                    }
                },
                Err(_) => 0.5,
            };

            let new_energy = current.energy * edge_weight * decay_factor;
            let old_energy = *energy_map.get(&target_id).unwrap_or(&0.0);
            
            if new_energy > old_energy && new_energy >= min_threshold {
                energy_map.insert(target_id, new_energy);
                queue.push_back(NodeEnergy { id: target_id, energy: new_energy });
            }
        }
    }

    for (node_id, energy) in energy_map {
        let rec = memgraph.result_record()?;
        rec.insert_int(c_str!("node_id"), node_id)?;
        rec.insert_double(c_str!("energy"), energy)?;
    }

    Ok(())
});

init_module!(|memgraph: &Memgraph| -> Result<()> {
    memgraph.add_read_procedure(
        optimize_context,
        c_str!("optimize_context"),
        &[
            define_type!("start_node_ids", Type::List, Type::Int),
            define_type!("decay_factor", Type::Number),
            define_type!("min_threshold", Type::Number)
        ],
        &[],
        &[
            define_type!("node_id", Type::Int),
            define_type!("energy", Type::Number)
        ]
    )?;
    Ok(())
});

close_module!(|| -> Result<()> { Ok(()) });
